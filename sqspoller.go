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
	ErrNoMessageHandler       = errors.New("ErrNoMessageHandler: no message handler set on poller instance")
	ErrNoErrorHandler         = errors.New("ErrNoErrorHandler: no error handler set on poller instance")
	ErrNoReceiveMessageParams = errors.New("ErrNoReceiveMessageParams: no ReceiveMessage parameters have been set")
	ErrHandlerTimeout         = errors.New("ErrHandlerTimeout: messageHandler took to long to process message")
	ErrShutdownNow            = errors.New("ErrShutdownNow: poller was suddenly shutdown")
	ErrShutdownGraceful       = errors.New("ErrShutdownGraceful: poller could not shutdown gracefully in time")
	ErrAlreadyShuttingDown    = errors.New("ErrAlreadyShuttingDown: poller is already in the process of shutting down")
	ErrAlreadyRunning         = errors.New("ErrAlreadyShuttingDown: poller is already running")
	ErrIntegrityIssue         = errors.New("ErrIntegrityIssue: unknown integrity issue")
)

// MessageHandler is a function which handles the incoming SQS message.
//
// The sqs Client used to instantiate the poller will also be made available to
// allow the user to perform standard sqs operations.
type MessageHandler func(ctx context.Context, client *sqs.SQS, msgOutput *MessageOutput) error

// ErrorHandler is a function which handlers errors returned from
// sqs.ReceiveMessageWithContext, it will only be invoked if the error is not
// nil. Returning nil from the ErrorHandler will allow the poller to continue,
// returning an error will cause the poller to exit.
//
// Errors should be of type awserr.Error, if the sqs.ReceiveMessageWithContext
// function returns the errors as expected.
type ErrorHandler func(ctx context.Context, err error) error

// ctxKey is the package's context key type used to store values on context.Context
// object to avoid clashing with other packages.
type ctxKey int

// TrackingKey should be used to access the values on the context object of type
// *TackingValue.
const TrackingKey ctxKey = 1

// TackingValue represents the values stored on the context object, for each poll
// the context object will store the time of message received and a trace ID.
type TackingValue struct {
	TraceID string
	Now     time.Time
}

// Poller is an instance of the polling framework, it contains the SQS client
// and provides a simple API for polling an SQS queue.
type Poller struct {
	client   *sqs.SQS
	queueURL string

	// Time to wait for messageHandler to process message, if messageHandler
	// function takes longer than this to return, then the program is exited.
	HandlerTimeout time.Duration

	// Holds the time of the last poll request that was made. This can be checked
	// periodically, to confirm the Poller is running as expected.
	LastPollTime time.Time
	// Maximum time interval between each poll when poll requests are returning
	// empty responses.
	IdlePollInterval time.Duration

	// Current poll interval, this interval will reach the IdlePollInterval
	// upon enough consecutive empty poll requests. Once a successful message
	// response is received, the CurrentInterval will drop back down to 0.
	CurrentInterval time.Duration

	errorHandler    ErrorHandler   // Handler used to handle message request errors
	messageHandler  MessageHandler // Handler used to handle successful message requests.
	outerMiddleware []Middleware   // Outer middleware of messageHandler.
	innerMiddleware []Middleware   // Inner middleware of messageHandler,

	// Active is true if the last poll returned a non empty message output.
	// While active, the PollInterval is ignored and the poller polls the
	// queue continuously without a pause until an empty response is returned
	// and active is set back to false.
	queueEmpty bool

	running        int64          // 1 if Poller is in running state, 0 if not.
	shuttingDown   int64          // 1 if Poller is in the process of shutting down, 0 if not.
	shutdown       chan *shutdown // channel to send shutdown instructions on.
	shutdownErrors chan error     // channel to send errors on shutdown.
	stopRequest    chan struct{}  // channel to send request to block polling
	stopConfirmed  chan struct{}  // channel to send confirmation that polling has been blocked
	exitWait       chan struct{}  // channel to send signal to exit waiting on poll interval.

	receiveMsgInput *sqs.ReceiveMessageInput // parameters to make message request to SQS.
	options         []request.Option         // request options.
}

// New creates a new instance of the SQS Poller from an instance of sqs.SQS.
func New(sqsSvc *sqs.SQS) *Poller {
	p := Poller{
		client: sqsSvc,

		shutdown:       make(chan *shutdown),
		shutdownErrors: make(chan error, 1),
		stopRequest:    make(chan struct{}, 1),
		stopConfirmed:  make(chan struct{}),
		exitWait:       make(chan struct{}, 1),

		outerMiddleware: make([]Middleware, 0),
	}

	return &p
}

// Default creates a new instance of the SQS Poller from an instance of sqs.SQS.
// It also comes set up with the recommend outerMiddleware plugged in.
func Default(sqsSvc *sqs.SQS) *Poller {
	p := New(sqsSvc)
	p.Use(IgnoreEmptyResponses())
	return p
}

// OnMessage attaches a MessageHandler to the Poller instance, if a MessageHandler
// already exists on the Poller instance, it will be replaced. The Middleware
// supplied to OnMessage will be applied first before any global middleware set
// by Use().
func (p *Poller) OnMessage(handler MessageHandler, middleware ...Middleware) {
	p.messageHandler = handler
	p.innerMiddleware = middleware
}

// OnError attaches an ErrorHandler to the Poller instance. It is the first line
// of defence against message request errors from SQS.
func (p *Poller) OnError(handler ErrorHandler) {
	p.errorHandler = handler
}

// Run starts the poller, the poller will continuously poll SQS until an error is
// returned, or explicitly told to shutdown.
func (p *Poller) Run() error {
	// Validate run
	if err := p.checkAndSetRunningStatus(); err != nil {
		return err
	}
	defer p.resetRunState()

	if p.messageHandler == nil {
		return ErrNoMessageHandler
	}
	if p.errorHandler == nil {
		return ErrNoErrorHandler
	}
	if p.receiveMsgInput == nil {
		return ErrNoReceiveMessageParams
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Apply middleware upon starting
	msgHandler := applyTimeout(p.messageHandler, p.HandlerTimeout)

	msgHandler = wrapMiddleware(msgHandler, p.innerMiddleware...)
	msgHandler = wrapMiddleware(msgHandler, p.outerMiddleware...)

	// Start polling
	pollingErrors := p.poll(ctx, msgHandler)

	// Handle polling errors and shutdown signals
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

// poll continuously polls the SQS queue in a separate goroutine, the errors are
// returned on the returned channel.
func (p *Poller) poll(ctx context.Context, msgHandler MessageHandler) <-chan error {

	errorChan := make(chan error)

	go func() {
		defer close(errorChan)
	polling:
		for {
			p.LastPollTime = time.Now()

			// add tracking info to context object
			v := TackingValue{TraceID: uuid.New().String(), Now: time.Now()}
			ctx = context.WithValue(ctx, TrackingKey, &v)

			// Make request to SQS queue for message.
			out, sqsErr := p.client.ReceiveMessageWithContext(ctx, p.receiveMsgInput, p.options...)

			// Handle ReceiveMessageWithContext results in separate goroutine.
			// and listen for errors on error channel.
			handlerErrors := p.handle(ctx, msgHandler, out, sqsErr)

			// Wait for msgHandler to handle message and check returned errors.
			if err := waitForError(ctx, handlerErrors); err != nil {
				errorChan <- err
				return
			}

			// handle polling back off if message responses from queue are empty.
			if err := p.handlePollInterval(ctx); err != nil {
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

// handle handles the results from the call to sqs.ReceiveMessageWithContext
// function, by passing the results through the message and error handlers. The
// function returns a channel for which the caller can listen on to receive the
// resulting error.
func (p *Poller) handle(ctx context.Context, msgHandler MessageHandler, out *sqs.ReceiveMessageOutput, sqsErr error) <-chan error {
	handlerErrors := make(chan error)

	go func() {
		defer close(handlerErrors)

		if sqsErr != nil {

			// call error handler is sqs error is not nil.
			if err := p.errorHandler(ctx, sqsErr); err != nil {

				// if error was not resolved in handler
				// then send error into channel and exit.
				handlerErrors <- err
				return
			}
		}

		// determine queue empty states from message output.
		p.queueEmpty = messageOutputIsEmpty(out)

		// handle message if there was no sqs error or if the error was resolved
		if err := msgHandler(ctx, p.client, convertMessage(out, p.client, p.queueURL)); err != nil {
			handlerErrors <- err
			return
		}
		handlerErrors <- nil
	}()

	return handlerErrors
}
