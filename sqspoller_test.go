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
	t.Run("default poller - ctx has TackingValue", Test.DefaultPollerContextValue)
	t.Run("race - shutdown", Test.RaceShutdown)
	t.Run("error handler - exit", Test.OnErrorExit)
	t.Run("error handler - continue", Test.OnErrorContinue)
	t.Run("setup errors", Test.SetupErrors)
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
	// Attach MessageHandler and Start Polling.
	// Assert that the correct message is received and that the correct
	// error is returned.
	confirmedRunning := errors.New("started and exited")

	msgHandler := func(ctx context.Context, client *sqs.SQS, msgOut *sqspoller.MessageOutput) error {
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

	errorHandler := func(ctx context.Context, err error) error {
		return err
	}

	poller.OnMessage(msgHandler)
	poller.OnError(errorHandler)

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
	// Set up empty handlerOnMsg.
	msgHandler := func(ctx context.Context, client *sqs.SQS, msgOut *sqspoller.MessageOutput) error {
		time.Sleep(time.Second)
		return nil
	}

	errorHandler := func(ctx context.Context, err error) error {
		return err
	}

	poller.OnMessage(msgHandler)
	poller.OnError(errorHandler)

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
	// Set up MessageHandler - start shutdown at the start of the
	// handling and make sure shutdown hasn't finished by the
	// time it returns.
	var confirmed bool
	shutdownFinished := make(chan error)

	msgHandler := func(ctx context.Context, client *sqs.SQS, msgOut *sqspoller.MessageOutput) error {

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
			t.Fatalf("shutdown should not have finished before handlerOnMsg returned")
		default:
		}

		return nil
	}

	errorHandler := func(ctx context.Context, err error) error {
		return err
	}

	poller.OnMessage(msgHandler)
	poller.OnError(errorHandler)

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
	// Set up empty handlerOnMsg.
	msgHandler := func(ctx context.Context, client *sqs.SQS, msgOut *sqspoller.MessageOutput) error {
		time.Sleep(200 * time.Millisecond)
		return nil
	}

	errorHandler := func(ctx context.Context, err error) error {
		return err
	}

	poller.OnMessage(msgHandler)
	poller.OnError(errorHandler)

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
	// Set up empty handlerOnMsg.
	msgHandler := func(ctx context.Context, client *sqs.SQS, msgOut *sqspoller.MessageOutput) error {
		time.Sleep(500 * time.Millisecond)
		return nil
	}

	errorHandler := func(ctx context.Context, err error) error {
		return err
	}

	poller.OnMessage(msgHandler)
	poller.OnError(errorHandler)

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
	// Set up MessageHandler - make sure handlerOnMsg runs for longer than HandlerTimeout
	msgHandler := func(ctx context.Context, client *sqs.SQS, msgOut *sqspoller.MessageOutput) error {
		time.Sleep(time.Second)
		return nil
	}

	errorHandler := func(ctx context.Context, err error) error {
		return err
	}

	poller.OnMessage(msgHandler)
	poller.OnError(errorHandler)

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
	// Set up MessageHandler
	msgHandler := func(ctx context.Context, client *sqs.SQS, msgOut *sqspoller.MessageOutput) error {
		return nil
	}

	errorHandler := func(ctx context.Context, err error) error {
		return err
	}

	poller.OnError(errorHandler)
	poller.OnMessage(msgHandler)

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
	// Set up MessageHandler - confirm that sqspoller.TackingValue is contained
	// within ctx object.

	msgHandler := func(ctx context.Context, client *sqs.SQS, msgOut *sqspoller.MessageOutput) error {
		_, ok := ctx.Value(sqspoller.TrackingKey).(*sqspoller.TackingValue)
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

	errorHandler := func(ctx context.Context, err error) error {
		return err
	}

	poller.OnMessage(msgHandler)
	poller.OnError(errorHandler)

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
	// Set up MessageHandler
	msgHandler := func(ctx context.Context, client *sqs.SQS, msgOut *sqspoller.MessageOutput) error {
		c.Broadcast()
		time.Sleep(time.Second)
		return nil
	}

	errorHandler := func(ctx context.Context, err error) error {
		return err
	}

	poller.OnMessage(msgHandler)
	poller.OnError(errorHandler)

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

func (p *PollerTests) OnErrorExit(t *testing.T) {
	poller := sqspoller.New(p.sqsClient)

	// ==============================================================
	// Supply invalid queue URL
	poller.ReceiveMessageParams(&sqs.ReceiveMessageInput{
		QueueUrl: aws.String("invalid-queue-url"),
	})

	msgHandler := func(ctx context.Context, client *sqs.SQS, msgOut *sqspoller.MessageOutput) error {
		return nil
	}

	errorHandler := func(ctx context.Context, err error) error {
		return err
	}

	poller.OnMessage(msgHandler)
	poller.OnError(errorHandler)

	err := poller.Run()
	if err == nil {
		t.Fatalf("expected error to be returned")
	}
}

func (p *PollerTests) OnErrorContinue(t *testing.T) {
	poller := sqspoller.New(p.sqsClient)

	// ==============================================================
	// Supply invalid queue URL
	poller.ReceiveMessageParams(&sqs.ReceiveMessageInput{
		QueueUrl: aws.String("invalid-queue-url"),
	})

	ErrContinued := errors.New("continued")
	msgHandler := func(ctx context.Context, client *sqs.SQS, msgOut *sqspoller.MessageOutput) error {
		return ErrContinued
	}

	var errorHandlerInvoked bool
	errorHandler := func(ctx context.Context, err error) error {
		errorHandlerInvoked = true
		return nil
	}

	poller.OnMessage(msgHandler)
	poller.OnError(errorHandler)

	err := poller.Run()
	if err != ErrContinued {
		t.Fatalf("unexpected error returned, expected ErrContinued, got %v", err)
	}

	if !errorHandlerInvoked {
		t.Fatalf("error handler shoudl have been invoked")
	}
}


func (p *PollerTests) SetupErrors(t *testing.T) {
	poller := sqspoller.New(p.sqsClient)

	if err := poller.Run(); err != sqspoller.ErrNoMessageHandler {
		t.Fatalf("unexpected error returned, wanted: ErrNoMessageHandler, got %v", err)
	}

	// attach message handler
	msgHandler := func(ctx context.Context, client *sqs.SQS, msgOut *sqspoller.MessageOutput) error {
		return nil
	}
	poller.OnMessage(msgHandler)

	if err := poller.Run(); err != sqspoller.ErrNoErrorHandler {
		t.Fatalf("unexpected error returned, wanted: ErrNoErrorHandler, got %v", err)
	}

	// attach error handler
	errorHandler := func(ctx context.Context, err error) error {
		return nil
	}
	poller.OnError(errorHandler)

	// leave off receive message params
	if err := poller.Run(); err != sqspoller.ErrNoReceiveMessageParams {
		t.Fatalf("unexpected error returned, wanted: ErrNoReceiveMessageParams, got %v", err)
	}
}