package sqspoller

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/google/uuid"
	"time"
)

var (
	ErrNoHandler = errors.New("ErrNoHandler: no handler set on Poller instance")

	ErrTimeoutNoMessages = errors.New("ErrTimeoutNoMessages: no new messages in given time frame")
	ErrTimeoutHandling   = errors.New("ErrTimeoutHandling: handler took to long to process message")
	ErrTimeoutShutdown   = errors.New("ErrTimeoutShutdown: could not shut down gracefully")

	ErrShutdownNow      = errors.New("ErrShutdownNow: poller was suddenly shutdown")
	ErrShutdownGraceful = errors.New("ErrShutdownGraceful: poller could not shutdown gracefully in time")

	ErrIntegrityIssue = errors.New("ErrIntegrityIssue: unknown integrity issue")
)

// ctxKey is the package's context key type used to store
// values on context.Context object to avoid clashing
// with other packages.
type ctxKey int

// CtxKey is the package's context key used to store
// values on context.Context object to avoid clashing with
// other packages.
const CtxKey ctxKey = 1

// CtxValue represents the values stored on the context
// object about the message response which is passed down
// through the handler function and middleware.
type CtxValue struct {
	TraceID string
	Now     time.Time
}

func newCtxValues(traceID string, t time.Time) *CtxValue {
	return &CtxValue{
		TraceID: traceID,
		Now:     t,
	}
}

// Handler is function which handles the incoming SQS
// message.
type Handler func(ctx context.Context, msgOutput *MessageOutput, err error) error

// Poller is an instance of the polling framework, it contains
// the SQS client and provides a simple API for polling an SQS
// queue.
type Poller struct {
	client   *sqs.SQS
	queueURL string

	// Time to wait for handler to process message, if handler function
	// takes longer than this to return, then the program is exited.
	TimeoutHandling time.Duration

	// Time interval between each poll request. After a poll request
	// has been made and response has been handled, the poller will
	// wait for this amount of time before making the next call.
	Interval time.Duration

	shutdown       chan *shutdown // channel to send shutdown instructions on.
	shutdownErrors chan error     // channel to send errors on shutdown.
	stopRequest    chan struct{}
	stopConfirmed  chan struct{}

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

		shutdown:       make(chan *shutdown),
		shutdownErrors: make(chan error, 1),
		stopRequest:    make(chan struct{}, 1),
		stopConfirmed:  make(chan struct{}),

		receiveMsgInput: &config,
		options:         options,
		middleware:      make([]Middleware, 0),

		ctx: context.Background(),
	}

	return &p
}

// Default creates a new instance of the SQS Poller from an instance
// of sqs.SQS and an sqs.ReceiveMessageInput, to configure how the
// SQS queue will be polled. It comes set up with the recommend middleware
// plugged in.
func Default(sqsSvc *sqs.SQS, config sqs.ReceiveMessageInput, options ...request.Option) *Poller {
	p := New(sqsSvc, config, options...)
	p.Use(IgnoreEmptyResponses())
	return p
}

// Handle attaches a Handler to the Poller instance, if a Handler already
// exists on the Poller instance, it will be replaced.
func (p *Poller) Handle(handler Handler, middleware ...Middleware) {
	handler = wrapMiddleware(middleware, handler)
	p.handler = handler
}

// Run starts the poller, the poller will continuously poll SQS until
// an error is returned, or explicitly told to shutdown.
func (p *Poller) Run() error {
	if p.handler == nil {
		return ErrNoHandler
	}

	ctx, cancel := context.WithCancel(p.ctx)

	//======================================================================
	// Apply Global Middleware upon starting
	handler := wrapMiddleware(p.middleware, p.handler)

	//======================================================================
	// Start Polling
	pollingErrors := p.poll(ctx, handler)

	//======================================================================
	// Handle Polling errors or shutdown signals
	for {
		select {
		case err := <-pollingErrors:
			if err != nil {
				return err
			}
		case sd := <-p.shutdown:
			return p.handleShutdown(sd, pollingErrors, cancel)
		}

	}
}

// poll continuously polls the SQS queue in a separate goroutine,
// the errors are returned on the returned channel.
func (p *Poller) poll(ctx context.Context, handler Handler) chan error {

	errorChan := make(chan error)

	go func() {
		defer close(errorChan)
	polling:
		for {
			//======================================================================
			// Make request to SQS queue for message
			out, err := p.client.ReceiveMessageWithContext(ctx, p.receiveMsgInput, p.options...)

			ctx := context.WithValue(ctx, CtxKey, newCtxValues(uuid.New().String(), time.Now()))

			//======================================================================
			// Call Handler with message request results.
			handlerError := make(chan error)
			go func() {
				if err := handler(ctx, convertMessage(out, p.client, p.queueURL), err); err != nil {
					handlerError <- err
					return
				}
				handlerError <- nil
			}()

			if err := p.waitOnHandling(ctx, handlerError); err != nil {
				errorChan <- err
				return
			}
			if err := p.waitForNextPoll(ctx); err != nil {
				errorChan <- err
				return
			}

			errorChan <- nil

			p.checkForStopRequests()

			continue polling

		}
	}()

	return errorChan
}

//// Error is the frameworks custom error type.
//type Error struct {
//	OriginalError error
//	Meta          map[string]interface{}
//	Message       string
//}
//
//// Error returns the error message.
//func (e *Error) Error() string {
//	return e.Message
//}
