// tests in this file require that you have docker installed and running.
package sqspoller_test

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/kinluek/sqspoller"
	"github.com/kinluek/sqspoller/internal/testing/setup"
	"sync"
	"testing"
	"time"
)

func TestPoller(t *testing.T) {
	svc, queueURL, teardown := setup.SQS(t)
	defer teardown()

	Test := PollerTests{svc, queueURL}

	t.Run("polling - basic", Test.BasicPolling)

	t.Run("shutdown - now", Test.ShutdownNow)
	t.Run("shutdown - gracefully", Test.ShutdownGracefully)
	t.Run("shutdown - after: time limit not reached", Test.ShutdownAfterLimitNotReached)
	t.Run("shutdown - after: time limit reached", Test.ShutdownAfterLimitReached)

	t.Run("timeout - handling", Test.HandlerTimeout)

	t.Run("LastPollTime updates", Test.LastPollTime)

	t.Run("default poller - ctx has CtxTackingValue", Test.DefaultPollerContextValue)

	t.Run("race - shutdown", Test.RaceShutdown)
}

// PollerTests holds the tests for the Poller
type PollerTests struct {
	sqsClient *sqs.SQS
	queueURL  *string
}

func (p *PollerTests) BasicPolling(t *testing.T) {
	// ==============================================================
	// Send message to SQS queue.
	messageBody := "message-body"

	sendResp, err := p.sqsClient.SendMessage(&sqs.SendMessageInput{
		QueueUrl:    p.queueURL,
		MessageBody: aws.String(messageBody),
	})
	if err != nil {
		t.Fatalf("failed to send message to SQS: %v", err)
	}

	// ==============================================================
	// Create new poller using local queue.
	poller := sqspoller.New(p.sqsClient)
	poller.ReceiveMessageParams(&sqs.ReceiveMessageInput{
		QueueUrl: p.queueURL,
	})

	// ==============================================================
	// Attach Handler and Start Polling.
	// Assert that the correct message is received and that the correct
	// error is returned.
	confirmedRunning := errors.New("started and exited")

	handler := func(ctx context.Context, msgOut *sqspoller.MessageOutput, err error) error {
		if len(msgOut.Messages) > 0 {
			if *msgOut.Messages[0].Body != messageBody {
				t.Fatalf("received message body: %v, wanted: %v", *msgOut.Messages[0].Body, messageBody)
			}
			if *msgOut.Messages[0].MessageId != *sendResp.MessageId {
				t.Fatalf("received message ID: %v, wanted: %v", *msgOut.Messages[0].MessageId, *sendResp.MessageId)
			}
			if _, err := msgOut.Messages[0].Delete(); err != nil {
				return err
			}
			return confirmedRunning
		}
		return confirmedRunning
	}

	poller.Handle(handler)

	err = poller.Run()
	if err != confirmedRunning {
		t.Fatalf("could not run poller: %v", err)
	}
}

func (p *PollerTests) ShutdownNow(t *testing.T) {
	// ==============================================================
	// Create new poller using local queue.
	poller := sqspoller.New(p.sqsClient)
	poller.ReceiveMessageParams(&sqs.ReceiveMessageInput{
		QueueUrl: p.queueURL,
	})

	// ==============================================================
	// Set up empty handler.
	handler := func(ctx context.Context, msgOut *sqspoller.MessageOutput, err error) error {
		time.Sleep(time.Second)
		return nil
	}

	poller.Handle(handler)

	pollingErrors := make(chan error, 1)

	// ==============================================================
	// Start Polling in separate goroutine
	go func() {
		pollingErrors <- nil
		pollingErrors <- poller.Run()
	}()

	<-pollingErrors // make sure poller.Run has been run before trying to shutdown

	if err := poller.ShutdownNow(); err != nil {
		t.Fatalf("error shutting down now: %v", err)
	}
	if err := <-pollingErrors; err != sqspoller.ErrShutdownNow {
		t.Fatalf("unexpected error shutting down now %v", err)
	}

	if err := poller.ShutdownNow(); err != nil {
		t.Fatalf("shutdown while poller not running should not return error: %v", err)
	}

}

func (p *PollerTests) ShutdownGracefully(t *testing.T) {
	// ==============================================================
	// Create new poller using local queue.
	poller := sqspoller.New(p.sqsClient)
	poller.ReceiveMessageParams(&sqs.ReceiveMessageInput{
		QueueUrl: p.queueURL,
	})

	// ==============================================================
	// Set up Handler - start shutdown at the start of the
	// handling and make sure shutdown hasn't finished by the
	// time it returns.
	var confirmed bool
	shutdownFinished := make(chan error)

	handler := func(ctx context.Context, msgOut *sqspoller.MessageOutput, err error) error {

		// use channels to confirm shutdown waits for
		// the requests to finish handling before shutting down.
		shuttingDown := make(chan struct{})
		go func() {
			shuttingDown <- struct{}{}
			shutdownFinished <- poller.ShutdownGracefully()
		}()

		<-shuttingDown

		time.Sleep(time.Second)
		confirmed = true

		select {
		case <-shutdownFinished:
			t.Fatalf("shutdown should not have finished before handler returned")
		default:
		}

		return nil
	}

	poller.Handle(handler)

	pollingErrors := make(chan error, 1)

	go func() {
		pollingErrors <- poller.Run()
	}()

	if err := <-shutdownFinished; err != nil {
		t.Fatalf("error shutting down %v", err)
	}

	if err := <-pollingErrors; err != nil {
		t.Fatalf("error polling %v", err)
	}
	if !confirmed {
		t.Fatalf("expected confirmed to be true, but got false")
	}

	if err := poller.ShutdownGracefully(); err != nil {
		t.Fatalf("shutdown while poller not running should not return error: %v", err)
	}

}

func (p *PollerTests) ShutdownAfterLimitNotReached(t *testing.T) {
	// ==============================================================
	// Create new poller using local queue.
	poller := sqspoller.New(p.sqsClient)
	poller.ReceiveMessageParams(&sqs.ReceiveMessageInput{
		QueueUrl: p.queueURL,
	})

	// ==============================================================
	// Set up empty handler.
	handler := func(ctx context.Context, msgOut *sqspoller.MessageOutput, err error) error {
		time.Sleep(200 * time.Millisecond)
		return nil
	}

	poller.Handle(handler)
	pollingErrors := make(chan error, 1)

	// ==============================================================
	// Start Polling in separate goroutine
	go func() {
		pollingErrors <- nil
		pollingErrors <- poller.Run()
	}()

	<-pollingErrors

	if err := poller.ShutdownAfter(time.Second); err != nil {
		t.Fatalf("could not shutdown gracefully: %v", err)
	}

	if err := <-pollingErrors; err != nil {
		t.Fatalf("unexpected error shutting down: %v", err)
	}

	if err := poller.ShutdownAfter(time.Second); err != nil {
		t.Fatalf("shutdown while poller not running should not return error: %v", err)
	}

}

func (p *PollerTests) ShutdownAfterLimitReached(t *testing.T) {
	// ==============================================================
	// Create new poller using local queue.
	poller := sqspoller.New(p.sqsClient)
	poller.ReceiveMessageParams(&sqs.ReceiveMessageInput{
		QueueUrl: p.queueURL,
	})

	// ==============================================================
	// Set up empty handler.
	handler := func(ctx context.Context, msgOut *sqspoller.MessageOutput, err error) error {
		time.Sleep(500 * time.Millisecond)
		return nil
	}

	poller.Handle(handler)
	pollingErrors := make(chan error, 1)

	// ==============================================================
	// Start Polling in separate goroutine
	go func() {
		pollingErrors <- nil
		pollingErrors <- poller.Run()
	}()

	<-pollingErrors

	if err := poller.ShutdownAfter(100 * time.Millisecond); err != sqspoller.ErrShutdownGraceful {
		t.Fatalf("unexpected error returned from ShutdownAfter(): %v", err)
	}

	if err := <-pollingErrors; err != sqspoller.ErrShutdownGraceful {
		t.Fatalf("unexpected error return from pollingErrors: %v", err)
	}

	if err := poller.ShutdownAfter(100 * time.Millisecond); err != nil {
		t.Fatalf("shutdown while poller not running should not return error: %v", err)
	}
}

func (p *PollerTests) HandlerTimeout(t *testing.T) {
	// ==============================================================
	// Create new poller using local queue.
	poller := sqspoller.New(p.sqsClient)
	poller.ReceiveMessageParams(&sqs.ReceiveMessageInput{
		QueueUrl: p.queueURL,
	})

	poller.SetHandlerTimeout(time.Millisecond)

	// ==============================================================
	// Set up Handler - make sure handler runs for longer than HandlerTimeout
	handler := func(ctx context.Context, msgOut *sqspoller.MessageOutput, err error) error {
		time.Sleep(time.Second)
		return nil
	}

	poller.Handle(handler)

	if err := poller.Run(); err != sqspoller.ErrHandlerTimeout {
		t.Fatalf("expected to get ErrHandlerTimeout, got %v", err)
	}
}

func (p *PollerTests) LastPollTime(t *testing.T) {
	// ==============================================================
	// Create new poller using local queue.
	poller := sqspoller.New(p.sqsClient)
	poller.ReceiveMessageParams(&sqs.ReceiveMessageInput{
		QueueUrl: p.queueURL,
	})

	// ==============================================================
	// Set up Handler
	handler := func(ctx context.Context, msgOut *sqspoller.MessageOutput, err error) error {
		return nil
	}
	poller.Handle(handler)

	// ==============================================================
	// Start Polling in separate goroutine
	pollingErrors := make(chan error)
	go func() {
		pollingErrors <- poller.Run()
	}()

	t1 := poller.LastPollTime
	time.Sleep(time.Second)

	t2 := poller.LastPollTime

	if t1.String() == t2.String() {
		t.Fatalf("t1: %v, should not equal t2: %v", t1.String(), t2.String())
	}

	poller.ShutdownNow()
}

func (p *PollerTests) DefaultPollerContextValue(t *testing.T) {
	// ==============================================================
	// Put a message in queue
	_, err := p.sqsClient.SendMessage(&sqs.SendMessageInput{
		QueueUrl:    p.queueURL,
		MessageBody: aws.String("messageBody"),
	})
	if err != nil {
		t.Fatalf("failed to send message to SQS: %v", err)
	}

	// ==============================================================
	// Create new poller using local queue.
	poller := sqspoller.Default(p.sqsClient)
	poller.ReceiveMessageParams(&sqs.ReceiveMessageInput{
		QueueUrl: p.queueURL,
	})

	// ==============================================================
	// Set up Handler - confirm that sqspoller.CtxTackingValue is contained
	// within ctx object.

	handler := func(ctx context.Context, msgOut *sqspoller.MessageOutput, err error) error {
		_, ok := ctx.Value(sqspoller.CtxKey).(*sqspoller.CtxTackingValue)
		if !ok {
			t.Fatalf("ctx should container CtxValues object")
		}
		go func() {
			if err := poller.ShutdownGracefully(); err != nil {
				t.Fatalf("could not shut down gracefully: %v", err)
			}
		}()
		return nil
	}

	poller.Handle(handler)
	if err := poller.Run(); err != nil {
		t.Fatalf("poller should not have returned error: %v", err)
	}

}

func (p *PollerTests) RaceShutdown(t *testing.T) {
	t.Skip("RaceShutdown: cannot guarantee race condition will be met")
	// ==============================================================
	// Create new poller using local queue.
	poller := sqspoller.New(p.sqsClient)
	poller.ReceiveMessageParams(&sqs.ReceiveMessageInput{
		QueueUrl: p.queueURL,
	})

	c := sync.NewCond(&sync.Mutex{})

	// ==============================================================
	// Set up Handler
	handler := func(ctx context.Context, msgOut *sqspoller.MessageOutput, err error) error {
		c.Broadcast()
		time.Sleep(time.Second)
		return nil
	}

	poller.Handle(handler)

	go func() {
		poller.Run()
	}()

	shutdownErrors := make(chan error, 2)

	var shutdownCalls sync.WaitGroup
	shutdownCalls.Add(2)

	go func() {
		defer shutdownCalls.Done()
		c.L.Lock()
		defer c.L.Unlock()
		c.Wait()
		shutdownErrors <- poller.ShutdownGracefully()
	}()
	go func() {
		defer shutdownCalls.Done()
		c.L.Lock()
		defer c.L.Unlock()
		c.Wait()
		shutdownErrors <- poller.ShutdownGracefully()
	}()

	shutdownCalls.Wait()
	close(shutdownErrors)

	var confirmedErr bool
	for err := range shutdownErrors {
		if err == sqspoller.ErrAlreadyShuttingDown {
			confirmedErr = true
		}
	}
	if !confirmedErr {
		t.Fatalf("unexpected error returned for shutdown, expected ErrAlreadyShuttingDown")
	}

}
