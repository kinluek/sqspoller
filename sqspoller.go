package sqspoller

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/sqs"
	"time"
)

var (
	ErrNoHandler         = errors.New("ErrNoHandler: no handler set on Poller instance")
	ErrTimeoutNoMessages = errors.New("ErrTimeoutNoMessages: no new messages in given time frame")
	ErrIntegrityIssue    = errors.New("ErrIntegrityIssue: unknown integrity issue")
)

// Handler is function which handles the incoming SQS
// message.
type Handler func(ctx context.Context, msgOutput *MessageOutput, err error) error

// Poller is an instance of the polling framework, it contains
// the SQS client and provides a simple API for polling an SQS
// queue.
type Poller struct {
	client   *sqs.SQS
	queueURL string

	AllowTimeout      bool          // If set to true, the timeouts are taken into effect, else, timeouts are ignored.
	TimeoutNoMessages time.Duration // Stop polling after the length of time since last message exceeds this value.
	TimeoutShutdown   time.Duration // Poller will try to shutdown gracefully up to this time limit, where the poller will exit regardless of what's happening.
	Interval          time.Duration // Time interval between each poll request.

	shutdown       chan struct{} // channel to send shutdown signal on
	shutdownErrors chan error    // channel to send errors on shutdown.

	handler         Handler
	middleware      []Middleware
	receiveMsgInput *sqs.ReceiveMessageInput
	options         []request.Option

	ctx context.Context
}

// New creates a new instance of the SQS Poller from an instance
// of sqs.SQS and an sqs.ReceiveMessageInput, to configure how the
// SQS queue will be polled.
func New(sqsSvc *sqs.SQS, config sqs.ReceiveMessageInput, options ...request.Option) *Poller {
	p := Poller{
		client: sqsSvc,

		queueURL: *config.QueueUrl,

		AllowTimeout: false,

		TimeoutShutdown: 5 * time.Second,
		shutdown:        make(chan struct{}),
		shutdownErrors:  make(chan error, 1),

		receiveMsgInput: &config,
		options:         options,
		middleware:      make([]Middleware, 0),

		ctx: context.Background(),
	}

	return &p
}

// Handle attaches a Handler to the Poller instance, if a Handler already
// exists on the Poller instance, it will be replaced.
func (p *Poller) Handle(handler Handler, middleware ...Middleware) {
	handler = wrapMiddleware(middleware, handler)
	handler = wrapMiddleware(p.middleware, handler)
	p.handler = handler
}

// StartPolling starts the poller.
func (p *Poller) StartPolling() error {
	if p.handler == nil {
		return ErrNoHandler
	}

	ctx, cancel := context.WithCancel(p.ctx)

	//======================================================================
	// Start Polling
	pollingErrors := p.poll(ctx)

	//======================================================================
	// Handle Polling errors or Shutdown signals
polling:
	for {
		select {
		case err := <-pollingErrors:
			if err != nil {
				return err
			}
		case <-p.shutdown:
			cancel()
			break polling
		}

	}

	//======================================================================
	// Flush out remaining errors after shutdown
	for err := range pollingErrors {
		if err == context.Canceled {
			p.shutdownErrors <- nil
			return nil
		}
		if err != nil {
			p.shutdownErrors <- err
			return err
		}
	}

	// This code should never be reached! Urgent fix
	// required if this error is ever returned!
	p.shutdownErrors <- ErrIntegrityIssue
	return nil
}

// poll continuously polls the SQS queue in a separate goroutine,
// the errors are returned on the returned channel.
func (p *Poller) poll(ctx context.Context) chan error {

	errorChan := make(chan error)

	go func() {

		timeout := time.After(p.TimeoutNoMessages)

	polling:
		for {
			var handlingMessage bool
			//======================================================================
			// Make request to SQS queue for message
			out, err := p.client.ReceiveMessageWithContext(ctx, p.receiveMsgInput, p.options...)

			if len(out.Messages) > 0 {
				handlingMessage = true
			}

			//======================================================================
			// Call Handler with message request results.
			handlerError := make(chan error)
			go func() {
				if err := p.handler(ctx, convertMessage(out, p.client, p.queueURL), err); err != nil {
					handlerError <- err
					return
				}
				handlerError <- nil
			}()

			//======================================================================
			// Wait for handler to return or handle cancellation.
			switch p.AllowTimeout {
			case false:
				if err := waitForSignals(ctx, handlerError, p.Interval); err != nil {
					errorChan <- err
					close(errorChan)
					return
				}
				continue polling
			case true:
				if err := waitForSignalsWithTimeout(ctx, handlerError, p.Interval, handlingMessage, timeout); err != nil {
					errorChan <- err
					close(errorChan)
					return
				}
				if handlingMessage {
					timeout = time.After(p.TimeoutNoMessages)
				}
				continue polling
			}

		}
	}()

	return errorChan
}

// Shutdown gracefully shuts down the poller.
func (p *Poller) Shutdown() error {
	p.shutdown <- struct{}{}
	return <-p.shutdownErrors
}

// Error is the frameworks custom error type.
type Error struct {
	OriginalError error
	Meta          map[string]interface{}
	Message       string
}

// Error returns the error message.
func (e *Error) Error() string {
	return e.Message
}
