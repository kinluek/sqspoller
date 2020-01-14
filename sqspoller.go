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
)

// Handler is function which handles the incoming SQS
// message.
type Handler func(ctx context.Context, msgOutput *MessageOutput, err error) error

// Poller is an instance of the polling framework, it contains
// the SQS client and provides a simple API for polling an SQS
// queue.
type Poller struct {
	client *sqs.SQS

	QueueURL string

	Interval          time.Duration // Time interval between each poll request.
	AllowTimeout      bool          // If set to true, the timeouts are taken into effect, else, timeouts are ignored.
	TimeoutNoMessages time.Duration // Stop polling after the length of time since last message exceeds this value.

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

		QueueURL: *config.QueueUrl,

		AllowTimeout: false,

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
	defer cancel()

	timeout := time.After(p.TimeoutNoMessages)

polling:
	for {
		var handlingMessage bool
		//======================================================================
		// Make message receive request
		out, err := p.client.ReceiveMessageWithContext(ctx, p.receiveMsgInput, p.options...)

		if len(out.Messages) > 0 {
			handlingMessage = true
		}

		handlerError := make(chan error)

		//======================================================================
		// Handler is called here
		go func() {
			if err := p.handler(ctx, convertMessage(out, p.client, p.QueueURL), err); err != nil {
				handlerError <- err
				return
			}
			handlerError <- nil
		}()

		//======================================================================
		// Wait for handler to finish
		if !p.AllowTimeout {
			if err := waitForSignals(ctx, handlerError, p.Interval); err != nil {
				return err
			}
			continue polling
		}

		if p.AllowTimeout {
			if err := waitForSignalsWithTimeout(ctx, handlerError, p.Interval, handlingMessage, timeout); err != nil {
				return err
			}
			if handlingMessage {
				timeout = time.After(p.TimeoutNoMessages)
			}
			continue polling
		}
		close(handlerError)

	}
	return nil
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
