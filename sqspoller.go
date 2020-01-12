package sqspoller

import (
	"context"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/sqs"
	"time"
)


// Poller is an instance of the polling framework, it contains
// the SQS client and provides a simple API for polling an SQS
// queue.
type Poller struct {
	client *sqs.SQS

	Interval time.Duration // time interval between each poll request - default: 10s.


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

		Interval: 10 * time.Second,

		receiveMsgInput: &config,
		options:         options,
		middleware:      make([]Middleware, 0),
	}
	return &p
}


// Handler is function which handles the incoming SQS
// message.
type Handler func(ctx context.Context, message *sqs.ReceiveMessageOutput, err error) error

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
		return &Error{Message: "no handler detected: please provide a handler."}
	}

	ctx := context.Background()

	for {
		out, err := p.client.ReceiveMessageWithContext(ctx, p.receiveMsgInput, p.options...)
		if err := p.handler(ctx, out, err); err != nil {
			return &Error{
				OriginalError: err,
				Meta:          nil,
				Message:       err.Error(),
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
