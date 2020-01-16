package sqspoller_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/kinluek/sqspoller"
	"github.com/kinluek/sqspoller/internal/testing/setup"
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

	t.Run("timeout - handling", Test.TimeoutHandling)
	t.Run("context -  contains CtxValue", Test.ContextValue)
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
	poller := sqspoller.New(p.sqsClient, sqs.ReceiveMessageInput{
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

	poller := sqspoller.New(p.sqsClient, sqs.ReceiveMessageInput{
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
		pollingErrors <- poller.Run()
	}()

	if err := poller.ShutdownNow(); err != nil {
		t.Fatalf("error shutting down now: %v", err)
	}
	if err := <-pollingErrors; err != sqspoller.ErrShutdownNow {
		t.Fatalf("unexpected error shutting down now %v", err)
	}

}

func (p *PollerTests) ShutdownGracefully(t *testing.T) {
	// ==============================================================
	// Create new poller using local queue.

	poller := sqspoller.New(p.sqsClient, sqs.ReceiveMessageInput{
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
		fmt.Println("shut down started")

		time.Sleep(time.Second)
		confirmed = true

		select {
		case <-shutdownFinished:
			t.Fatalf("shutdown should not have finished before handler returned")
		default:
		}

		fmt.Println("handler exited")
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

}

func (p *PollerTests) ShutdownAfterLimitNotReached(t *testing.T) {
	// ==============================================================
	// Create new poller using local queue.

	poller := sqspoller.New(p.sqsClient, sqs.ReceiveMessageInput{
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
		pollingErrors <- poller.Run()
	}()

	if err := poller.ShutdownAfter(time.Second); err != nil {
		t.Fatalf("could not shutdown gracefully: %v", err)
	}

	if err := <-pollingErrors; err != nil {
		t.Fatalf("unexpected error shutting down: %v", err)
	}

}

func (p *PollerTests) ShutdownAfterLimitReached(t *testing.T) {
	// ==============================================================
	// Create new poller using local queue.

	poller := sqspoller.New(p.sqsClient, sqs.ReceiveMessageInput{
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
		pollingErrors <- poller.Run()
	}()

	if err := poller.ShutdownAfter(100 * time.Millisecond); err != sqspoller.ErrShutdownGraceful {
		t.Fatalf("unexpected error returned from ShutdownAfter(): %v", err)
	}

	if err := <-pollingErrors; err != sqspoller.ErrShutdownGraceful {
		t.Fatalf("unexpected error return from pollingErrors: %v", err)
	}

}

func (p *PollerTests) TimeoutHandling(t *testing.T) {
	// ==============================================================
	// Create new poller using local queue.

	poller := sqspoller.New(p.sqsClient, sqs.ReceiveMessageInput{
		QueueUrl: p.queueURL,
	})

	poller.SetTimeoutHandling(time.Millisecond)

	// ==============================================================
	// Set up Handler - make sure handler runs for longer than TimeoutHandling
	handler := func(ctx context.Context, msgOut *sqspoller.MessageOutput, err error) error {
		time.Sleep(time.Second)
		return nil
	}

	poller.Handle(handler)

	if err := poller.Run(); err != sqspoller.ErrTimeoutHandling {
		t.Fatalf("expected to get ErrTimeoutHandling, got %v", err)
	}
}

func (p *PollerTests) ContextValue(t *testing.T) {
	// ==============================================================
	// Create new poller using local queue.

	poller := sqspoller.New(p.sqsClient, sqs.ReceiveMessageInput{
		QueueUrl: p.queueURL,
	})

	// ==============================================================
	// Set up Handler - confirm that sqspoller.CtxValue is contained
	// within ctx object.

	handler := func(ctx context.Context, msgOut *sqspoller.MessageOutput, err error) error {
		_, ok := ctx.Value(sqspoller.CtxKey).(*sqspoller.CtxValue)
		if !ok {
			t.Fatalf("ctx should container CtxValues object")
		}
		go func() {
			poller.ShutdownGracefully()
		}()
		return nil
	}

	poller.Handle(handler)
	if err := poller.Run(); err != nil {
		t.Fatalf("poller should not have returned error: %v", err)
	}

}

