package sqspoller

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/google/uuid"
	"sync"
	"time"
)

var (
	// ErrNoMessageHandler occurs when the caller tries to run the poller before attaching a MessageHandler.
	ErrNoMessageHandler = errors.New("ErrNoMessageHandler: no message handler set on poller instance")

	// ErrNoErrorHandler occurs when the caller tries to run the poller before attaching an ErrorHandler.
	ErrNoErrorHandler = errors.New("ErrNoErrorHandler: no error handler set on poller instance")

	// ErrNoReceiveMessageParams occurs when the caller tries to run the poller before setting the ReceiveMessageParams.
	ErrNoReceiveMessageParams = errors.New("ErrNoReceiveMessageParams: no ReceiveMessage parameters have been set")

	// ErrHandlerTimeout occurs when the MessageHandler times out before processing the message.
	ErrHandlerTimeout = errors.New("ErrHandlerTimeout: message handler took to long to process message")

	// ErrRequestTimeout occurs when the poller times out while requesting for a message off the SQS queue.
	ErrRequestTimeout = errors.New("ErrRequestTimeout: requesting message from queue timed out")

	// ErrShutdownNow occurs when the poller is suddenly shutdown.
	ErrShutdownNow = errors.New("ErrShutdownNow: poller was suddenly shutdown")

	// ErrShutdownGraceful occurs when the poller fails to shutdown gracefully.
	ErrShutdownGraceful = errors.New("ErrShutdownGraceful: poller could not shutdown gracefully in time")

	// ErrNotCloseable occurs when the caller tries to shut down the poller is already stopped or in the process of shutting down.
	ErrNotCloseable = errors.New("ErrNotCloseable: poller is either stopped or already shutting down")

	// ErrNotRunnable occurs when the caller tries to run the poller while the poller is already running or shutting down.
	ErrNotRunnable = errors.New("ErrNotRunnable: poller is either already running or shutting down")

	// ErrNotRunnable occurs when there is an integrity issue in the system.
	ErrIntegrityIssue = errors.New("ErrIntegrityIssue: unknown integrity issue")
)

const (
	// the default request timeout
	defaultRequestTimeout = 30 * time.Second
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
// *TrackingValue.
const TrackingKey ctxKey = 1

// TrackingValue represents the values stored on the context object, for each poll
// the context object will store the time of message received and a trace ID.
type TrackingValue struct {
	TraceID string
	Now     time.Time
}

// Poller is an instance of the polling framework, it contains the SQS client
// and provides a simple API for polling an SQS queue.
type Poller struct {
	mu sync.Mutex

	client   *sqs.SQS
	queueURL string

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

	// Timeout on requesting for a new message from SQS. By default, this will
	// be 30 seconds, if it has not been set manually.
	RequestTimeout time.Duration

	// Time to wait for messageHandler to process message, if messageHandler
	// function takes longer than this to return, then the program is exited.
	// The timeout only takes into account the time for the core message handler
	// to complete, it does not include the time added for middleware.
	handlerTimeout time.Duration

	errorHandler    ErrorHandler   // Handler used to handle message request errors
	messageHandler  MessageHandler // Handler used to handle successful message requests.
	outerMiddleware []Middleware   // Outer middleware of messageHandler.
	innerMiddleware []Middleware   // Inner middleware of messageHandler,

	// queueEmpty is true if the last poll returned an non empty message output.
	// While the queue is empty, the CurrentInterval will increase exponentially
	// with each consecutive poll request until it reachs the IdlePollInterval
	// duration.
	queueEmpty bool

	// runStatus tells us the running status of the poller, 0 for off, 1 for running
	// and 2 for shutting down.
	runStatus      int32
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
	defer p.resetRunStatus()
	if err := p.validateSetup(); err != nil {
		return err
	}

	// Apply middleware upon starting.
	// Apply the timeout as the innermost middleware, so that timeout errors
	// can be caught by custom middleware to stop the poller from exiting.
	msgHandler := applyTimeout(p.messageHandler, p.handlerTimeout)
	msgHandler = wrapMiddleware(msgHandler, p.innerMiddleware...)
	msgHandler = wrapMiddleware(msgHandler, p.outerMiddleware...)

	// Start polling
	pollingErrors := p.poll(msgHandler)

	// Handle polling errors and shutdown signals
	for {
		select {
		case err := <-pollingErrors:
			if err != nil {
				return err
			}
		case sd := <-p.shutdown:
			return p.handleShutdown(sd, pollingErrors)
		}

	}
}

// poll continuously polls the SQS queue in a separate goroutine, the errors are
// returned on the returned channel.
func (p *Poller) poll(msgHandler MessageHandler) <-chan error {

	// Add buffer of one, so that the polling cycle can finish if the error is
	// not collected. Without the buffer, this may leak a blocked goroutine if
	// the poller exits due to a non-graceful shutdown.
	pollingErrors := make(chan error, 1)

	go func() {
		defer close(pollingErrors)

		for {
			p.LastPollTime = time.Now()

			// add tracking info to context object
			v := TrackingValue{TraceID: uuid.New().String(), Now: time.Now()}
			ctx := context.WithValue(context.Background(), TrackingKey, &v)

			// Make request to SQS queue for message.
			out, sqsErr := p.receiveMessage(ctx)

			// Handle the results returned from the request for new messages
			// from the SQS queue.
			if err := p.handle(ctx, msgHandler, out, sqsErr); err != nil {
				pollingErrors <- err
				return
			}

			// Handle poll interval, back off the wait time if message responses
			// from queue are empty.
			if err := p.handlePollInterval(); err != nil {
				pollingErrors <- err
				return
			}

			// Remember to return nil on the channel if everything was successful.
			pollingErrors <- nil

			// Stop polling if stop request has been received.
			if p.stopRequestReceived() {
				break
			}
		}
	}()

	return pollingErrors
}

// receiveMessage applies the request timeout to the context object calling the
// sqs.ReceiveMessageWithContext function with the receive message parameters.
func (p *Poller) receiveMessage(ctx context.Context) (*sqs.ReceiveMessageOutput, error) {

	// Set the request timeout on the context object, before making the request
	// to receive the message from the queue.
	ctx, cancel := context.WithTimeout(ctx, p.RequestTimeout)
	defer cancel()

	out, err := p.client.ReceiveMessageWithContext(ctx, p.receiveMsgInput, p.options...)
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {

			// If the function errored out due to the timeout, then
			// return ErrRequestTimeout to simplify error assertions
			// for the caller.
			if awsErr.OrigErr() == context.DeadlineExceeded {
				return nil, ErrRequestTimeout
			}
		}
	}
	return out, err
}

// handle handles the results from the call to sqs.ReceiveMessageWithContext
// function, by passing the results through the message and error handlers.
func (p *Poller) handle(ctx context.Context, msgHandler MessageHandler, out *sqs.ReceiveMessageOutput, sqsErr error) error {
	if sqsErr != nil {

		// call error handler if sqs error is not nil.
		if err := p.errorHandler(ctx, sqsErr); err != nil {
			return err
		}
	}

	// determine queue empty states from message output.
	p.queueEmpty = messageOutputIsEmpty(out)

	// handle message if there was no sqs error or if the error was resolved
	if err := msgHandler(ctx, p.client, convertMessage(out, p.client, p.queueURL)); err != nil {
		return err
	}

	return nil
}
