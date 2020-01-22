package sqspoller

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/sqs"
	"time"
)

var (
	ErrNoMessageHandler       = errors.New("ErrNoMessageHandler: no message handler set on poller instance")
	ErrNoErrorHandler         = errors.New("ErrNoErrorHandler: no error handler set on poller instance")
	ErrNoReceiveMessageParams = errors.New("ErrNoReceiveMessageParams: no ReceiveMessage parameters have been set")
	ErrHandlerTimeout         = errors.New("ErrHandlerTimeout: handlerOnMsg took to long to process message")
	ErrShutdownNow            = errors.New("ErrShutdownNow: poller was suddenly shutdown")
	ErrShutdownGraceful       = errors.New("ErrShutdownGraceful: poller could not shutdown gracefully in time")
	ErrAlreadyShuttingDown    = errors.New("ErrAlreadyShuttingDown: poller is already in the process of shutting down")
	ErrAlreadyRunning         = errors.New("ErrAlreadyShuttingDown: poller is already running")
	ErrIntegrityIssue         = errors.New("ErrIntegrityIssue: unknown integrity issue")
)

// MessageHandler is a function which handles the incoming
// SQS message.
//
// The sqs Client used to instantiate the poller will also
// be made available to allow the user to perform standard
// sqs operations.
type MessageHandler func(ctx context.Context, client *sqs.SQS, msgOutput *MessageOutput) error

// ErrorHandler is a function which handlers errors returned
// from sqs.ReceiveMessageWithContext. Returning nil from the
// ErrorHandler will allow the poller to continue, returning
// an error will cause the poller to exit.
//
// Errors should be of type *awserr.Error, if the
// sqs.ReceiveMessageWithContext returns the errors as expected.
type ErrorHandler func(ctx context.Context, err error) error

// Poller is an instance of the polling framework, it contains
// the SQS client and provides a simple API for polling an SQS
// queue.
type Poller struct {
	client   *sqs.SQS
	queueURL string

	// Time to wait for handlerOnMsg to process message, if handlerOnMsg
	// function takes longer than this to return, then the program is exited.
	HandlerTimeout time.Duration

	// Time interval between each poll request. After a poll request has been
	// made and response has been handled, the poller will wait for this amount
	// of time before making the next call.
	PollInterval time.Duration

	// Holds the time of the last poll request that was made. This can
	// be checked periodically, to confirm the Poller is running as expected.
	LastPollTime time.Time

	running        int64          // 1 if Poller is in running state, 0 if not.
	shuttingDown   int64          // 1 if Poller is in the process of shutting down, 0 if not.
	shutdown       chan *shutdown // channel to send shutdown instructions on.
	shutdownErrors chan error     // channel to send errors on shutdown.
	stopRequest    chan struct{}  // channel to send request to block polling
	stopConfirmed  chan struct{}  // channel to send confirmation that polling has been blocked

	handlerOnErr ErrorHandler // Handler used to handle message request errors

	handlerOnMsg    MessageHandler // Handler used to handle successful message requests.
	outerMiddleware []Middleware   // Outer middleware of handlerOnMsg.
	innerMiddleware []Middleware   // Inner middleware of handlerOnMsg,

	receiveMsgInput *sqs.ReceiveMessageInput // parameters to make message request to SQS.
	options         []request.Option         // request options.
}

// New creates a new instance of the SQS Poller from an instance
// of sqs.SQS.
func New(sqsSvc *sqs.SQS) *Poller {
	p := Poller{
		client: sqsSvc,

		shutdown:       make(chan *shutdown),
		shutdownErrors: make(chan error, 1),
		stopRequest:    make(chan struct{}, 1),
		stopConfirmed:  make(chan struct{}),

		outerMiddleware: make([]Middleware, 0),
	}

	return &p
}

// Default creates a new instance of the SQS Poller from an instance
// of sqs.SQS. It also comes set up with the recommend outerMiddleware
// plugged in.
func Default(sqsSvc *sqs.SQS) *Poller {
	p := New(sqsSvc)
	p.Use(IgnoreEmptyResponses())
	p.Use(Tracking())
	return p
}

// OnMessage attaches a MessageHandler to the Poller instance, if a
// MessageHandler already exists on the Poller instance, it will be
// replaced. The Middleware supplied to OnMessage will be applied first
// before any global middleware set by Use().
func (p *Poller) OnMessage(handler MessageHandler, middleware ...Middleware) {
	p.handlerOnMsg = handler
	p.innerMiddleware = middleware
}

// OnError attaches an ErrorHandler to the Poller instance. It is the
// first line of defence against message request errors from SQS.
func (p *Poller) OnError(handler ErrorHandler) {
	p.handlerOnErr = handler
}

// Run starts the poller, the poller will continuously poll SQS until
// an error is returned, or explicitly told to shutdown.
func (p *Poller) Run() error {
	//======================================================================
	// Validate Run
	if err := p.checkAndSetRunningStatus(); err != nil {
		return err
	}
	defer p.resetRunState()

	if p.handlerOnMsg == nil {
		return ErrNoMessageHandler
	}
	if p.handlerOnErr == nil {
		return ErrNoErrorHandler
	}
	if p.receiveMsgInput == nil {
		return ErrNoReceiveMessageParams
	}

	ctx, cancel := context.WithCancel(context.Background())

	//======================================================================
	// Apply Middleware upon starting
	handler := applyTimeout(p.handlerOnMsg, p.HandlerTimeout)

	handler = wrapMiddleware(handler, p.innerMiddleware...)
	handler = wrapMiddleware(handler, p.outerMiddleware...)

	//======================================================================
	// Start Polling
	pollingErrors := p.poll(ctx, handler)

	//======================================================================
	// OnMessage Polling errors, shutdown signals, heartbeats
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
func (p *Poller) poll(ctx context.Context, handler MessageHandler) chan error {

	errorChan := make(chan error)

	go func() {
		defer close(errorChan)
	polling:
		for {
			p.LastPollTime = time.Now()
			//======================================================================
			// Make request to SQS queue for message
			out, sqsErr := p.client.ReceiveMessageWithContext(ctx, p.receiveMsgInput, p.options...)

			//======================================================================
			// Handle ReceiveMessageWithContext results
			handlerError := make(chan error)
			go func() {
				if sqsErr != nil {
					// call error handler is sqs error is not nil.
					if err := p.handlerOnErr(ctx, sqsErr); err != nil {
						// if error was not resolved in handler
						// then send error into channel and exit.
						handlerError <- err
						return
					}
				}
				// handle message if there was no sqs error or if the error was resolved
				if err := handler(ctx, p.client, convertMessage(out, p.client, p.queueURL)); err != nil {
					handlerError <- err
					return
				}
				handlerError <- nil
			}()

			if err := waitForError(ctx, handlerError); err != nil {
				errorChan <- err
				return
			}
			if err := waitForInterval(ctx, p.PollInterval); err != nil {
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
