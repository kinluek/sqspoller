package sqspoller

import (
	"context"
	"errors"
	"fmt"
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

	Interval          time.Duration // Time interval between each poll request - default: 10s.
	AllowTimeout      bool          // If set to true, the timeouts are taken into effect, else, timeouts are ignored.
	TimeoutNoMessages time.Duration // Stop polling after the length of time since last message exceeds this value.

	receiveMsgInput *sqs.ReceiveMessageInput
	options         []request.Option
	handler         Handler
	middleware      []Middleware
}

// New creates a new instance of the SQS Poller from an instance
// of sqs.SQS and an sqs.ReceiveMessageInput, to configure how the
// SQS queue will be polled.
func New(sqsSvc *sqs.SQS, config sqs.ReceiveMessageInput, options ...request.Option) *Poller {
	p := Poller{
		client: sqsSvc,

		QueueURL: *config.QueueUrl,

		Interval:     10 * time.Second,
		AllowTimeout: false,

		receiveMsgInput: &config,
		options:         options,
		middleware:      make([]Middleware, 0),
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
		return &Error{
			OriginalError: ErrNoHandler,
			Message:       ErrNoHandler.Error(),
		}
	}

	ctx := context.Background()

	timeout := time.After(p.TimeoutNoMessages)
poll:
	for {
		//======================================================================
		// Make message receive request
		out, err := p.client.ReceiveMessageWithContext(ctx, p.receiveMsgInput, p.options...)

		//======================================================================
		// Set times
		interval := time.After(p.Interval)
		if len(out.Messages) > 0 {
			timeout = time.After(p.TimeoutNoMessages)
		}

		//======================================================================
		// Handler is called here
		if err := p.handler(ctx, messageOutput(out, p.client, p.QueueURL), err); err != nil {
			return &Error{
				OriginalError: err,
				Message:       err.Error(),
			}
		}

		//======================================================================
		// Handle intervals and timeouts
		for {
			select {
			case <-interval:
				continue poll
			case <-timeout:
				if p.AllowTimeout {
					return &Error{
						OriginalError: ErrTimeoutNoMessages,
						Message:       fmt.Sprintf("%v: %v", ErrTimeoutNoMessages, p.TimeoutNoMessages),
					}
				}
			}
		}

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
