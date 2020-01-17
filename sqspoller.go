package sqspoller

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/sqs"
	"time"
)

var (
	ErrNoHandler = errors.New("ErrNoHandler: no handler set on Poller instance")

	ErrHandlerTimeout = errors.New("ErrHandlerTimeout: handler took to long to process message")

	ErrShutdownNow      = errors.New("ErrShutdownNow: poller was suddenly shutdown")
	ErrShutdownGraceful = errors.New("ErrShutdownGraceful: poller could not shutdown gracefully in time")

	ErrIntegrityIssue = errors.New("ErrIntegrityIssue: unknown integrity issue")
)

// Handler is a function which handles the incoming SQS
// message.
//
// When making Handlers to be used by the Poller, make
// sure the error value is checked first, before any
// business logic code, unless you have created an error
// checking middleware that wraps the core Handler.
//
// If the error is non-nil, it will be of type *awserr.Error
// which is returned from a failed request for message from
// SQS.
type Handler func(ctx context.Context, msgOutput *MessageOutput, err error) error

// Poller is an instance of the polling framework, it contains
// the SQS client and provides a simple API for polling an SQS
// queue.
type Poller struct {
	client   *sqs.SQS
	queueURL string

	// Time to wait for handler to process message, if handler function
	// takes longer than this to return, then the program is exited.
	HandlerTimeout time.Duration

	// Time interval between each poll request. After a poll request
	// has been made and response has been handled, the poller will
	// wait for this amount of time before making the next call.
	PollInterval time.Duration

	// Holds the time of the last poll request that was made. This can
	// be checked periodically, to confirm the Poller is running as expected.
	LastPollTime time.Time

	running        bool           // true if Poller is in running state.
	shutdown       chan *shutdown // channel to send shutdown instructions on.
	shutdownErrors chan error     // channel to send errors on shutdown.
	stopRequest    chan struct{}  // channel to send request to block polling
	stopConfirmed  chan struct{}  // channel to send confirmation that polling has been blocked

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
	p.Use(Tracking())
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
	p.running = true
	defer func() {
		p.running = false
	}()

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
	// Handle Polling errors, shutdown signals, heartbeats
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
			p.LastPollTime = time.Now()
			//======================================================================
			// Make request to SQS queue for message
			out, err := p.client.ReceiveMessageWithContext(ctx, p.receiveMsgInput, p.options...)

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
