package sqspoller

import (
	"context"
	"github.com/google/uuid"
	"time"
)

// Middleware is a function which that wraps a Handler
// to add functionality before or after the Handler code.
type Middleware func(Handler) Handler

// Use attaches global middleware to the Poller instance which will
// wrap any Handler and Handler specific middleware.
func (p *Poller) Use(middleware ...Middleware) {
	if p.middleware == nil {
		p.middleware = middleware
	} else {
		p.middleware = append(p.middleware, middleware...)
	}
}

// wrapMiddleware creates a new handler by wrapping middleware around a final
// handler. The middlewares' Handlers will be executed by requests in the order
// they are provided.
func wrapMiddleware(middleware []Middleware, handler Handler) Handler {

	// start wrapping the handler from the end of the
	// middleware slice, to the start, this will ensure
	// the code is executed in the right order when, the
	// resulting handler is executed.
	for i := len(middleware) - 1; i >= 0; i-- {
		mw := middleware[i]
		if mw != nil {
			handler = mw(handler)
		}
	}

	return handler
}

// ctxKey is the package's context key type used to store
// values on context.Context object to avoid clashing with
// other packages.
type ctxKey int

// CtxKey should be used to access the values on the context
// object of type *CtxTackingValue.
//
// This can only be used if the Tracking middleware has been
// used. The Poller returned by Default() comes with this
// middleware installed.
const CtxKey ctxKey = 1

// CtxTackingValue represents the values stored on the
// context object about the message response which is passed
// down through the handler function and middleware.
//
// This can only be used if the Tracking middleware has been
// used. The Poller returned by Default() comes with this
// middleware installed.
type CtxTackingValue struct {
	TraceID string
	Now     time.Time
}

// Tracking adds tracking information to the context object for each
// message output received from the queue. The information can be
// accessed on the context object by using the CtxKey constant and
// returns a *CtxTackingValue object, containing a traceID and receive
// time.
func Tracking() Middleware {

	f := func(handler Handler) Handler {

		h := func(ctx context.Context, msgOutput *MessageOutput, err error) error {
			v := &CtxTackingValue{
				TraceID: uuid.New().String(),
				Now:     time.Now(),
			}
			ctx = context.WithValue(ctx, CtxKey, v)

			return handler(ctx, msgOutput, err)
		}

		return h
	}

	return f
}

// IgnoreEmptyResponses stops the data from being passed down
// to the inner handler, if there is no message to be handled.
func IgnoreEmptyResponses() Middleware {

	f := func(handler Handler) Handler {

		// new handler
		h := func(ctx context.Context, msgOutput *MessageOutput, err error) error {

			// validate messages exist, if no messages exist, do
			// not pass down the output and return nil
			if err == nil && len(msgOutput.Messages) == 0 || msgOutput.Messages == nil {
				return nil
			}

			return handler(ctx, msgOutput, err)
		}

		return h
	}

	return f
}

// HandlerTimeout takes a timeout duration and returns ErrHandlerTimeout if
// the handler cannot process the message within that time. The user can then
// use other middleware to check for ErrHandlerTimeout and decide whether to
// exit or move onto the next poll request.
func HandlerTimeout(t time.Duration) Middleware {

	f := func(handler Handler) Handler {

		h := func(ctx context.Context, msgOut *MessageOutput, err error) error {
			ctx, cancel := context.WithCancel(ctx)

			timer := time.NewTimer(t)
			defer timer.Stop()

			handlerErrors := make(chan error)
			go func() {
				handlerErrors <- handler(ctx, msgOut, err)
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
