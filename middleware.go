package sqspoller

import (
	"context"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/google/uuid"
	"time"
)

// Middleware is a function which that wraps a MessageHandler to add functionality
// before or after the MessageHandler code.
type Middleware func(MessageHandler) MessageHandler

// Use attaches global outerMiddleware to the Poller instance which will wrap
// any  MessageHandler and MessageHandler specific outerMiddleware.
func (p *Poller) Use(middleware ...Middleware) {
	if p.outerMiddleware == nil {
		p.outerMiddleware = middleware
	} else {
		p.outerMiddleware = append(p.outerMiddleware, middleware...)
	}
}

// wrapMiddleware creates a new messageHandler by wrapping outerMiddleware around
// a final messageHandler. The middlewares' Handlers will be executed by requests
// in the order they are provided.
func wrapMiddleware(handler MessageHandler, middleware ...Middleware) MessageHandler {

	// start wrapping the messageHandler from the end of the outerMiddleware
	// slice, to the start, this will ensure the code is executed in the right
	// order when, the resulting messageHandler is executed.
	for i := len(middleware) - 1; i >= 0; i-- {
		mw := middleware[i]
		if mw != nil {
			handler = mw(handler)
		}
	}

	return handler
}

// applyTimeout applies a timeout to the messageHandler if the timeout is greater
// than 0. If timeout is 0, then the function returns the messageHandler unchanged.
func applyTimeout(handler MessageHandler, timeout time.Duration) MessageHandler {
	if timeout > 0 {
		handler = wrapMiddleware(handler, HandlerTimeout(timeout))
	}
	return handler
}

// HandlerTimeout takes a timeout duration and returns ErrHandlerTimeout if the
// messageHandler cannot process the message within that time. The user can then
// use other outerMiddleware to check for ErrHandlerTimeout and decide whether to
// exit or move onto the next poll request.
func HandlerTimeout(t time.Duration) Middleware {

	f := func(handler MessageHandler) MessageHandler {

		h := func(ctx context.Context, client *sqs.SQS, msgOut *MessageOutput) error {
			ctx, cancel := context.WithCancel(ctx)

			timer := time.NewTimer(t)
			defer timer.Stop()

			handlerErrors := make(chan error)
			go func() {
				handlerErrors <- handler(ctx, client, msgOut)
			}()

			select {
			case err := <-handlerErrors:
				if err != nil {
					return err
				}
			case <-timer.C:
				cancel()
				return ErrHandlerTimeout
			}

			return nil
		}

		return h
	}

	return f
}

// ctxKey is the package's context key type used to store values on context.Context
// object to avoid clashing with other packages.
type ctxKey int

// TrackingKey should be used to access the values on the context object of type
// *TackingValue placed by the Tacking middleware.
const TrackingKey ctxKey = 1

// TackingValue represents the values stored on the context object placed by the
// Tracking middeware.
//
// The Poller returned by Default() comes with this middleware installed.
type TackingValue struct {
	TraceID string
	Now     time.Time
}

// Tracking adds tracking information to the context object for each message output
// received from the queue. The information can be accessed on the context object
// by using the TrackingKey constant and returns a *TackingValue object, containing
// a traceID and receive time.
func Tracking() Middleware {

	f := func(handler MessageHandler) MessageHandler {

		h := func(ctx context.Context, client *sqs.SQS, msgOutput *MessageOutput) error {

			// add tracking info to context object
			v := &TackingValue{
				TraceID: uuid.New().String(),
				Now:     time.Now(),
			}
			ctx = context.WithValue(ctx, TrackingKey, v)

			return handler(ctx, client, msgOutput)
		}

		return h
	}

	return f
}

// IgnoreEmptyResponses stops the data from being passed down to the inner
// messageHandler, if there is no message to be handled.
func IgnoreEmptyResponses() Middleware {

	f := func(handler MessageHandler) MessageHandler {

		h := func(ctx context.Context, client *sqs.SQS, msgOutput *MessageOutput) error {

			// validate messages exist, if no messages exist, do
			// not pass down the output and return nil
			if msgOutput.Messages == nil || len(msgOutput.Messages) == 0 {
				return nil
			}

			return handler(ctx, client, msgOutput)
		}

		return h
	}

	return f
}
