package sqspoller

import "context"

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


// IgnoreEmptyResponses stops the data from being passed down
// to the inner handler, if there is no message to be handled.
func IgnoreEmptyResponses() Middleware {
	f := func(handler Handler) Handler {

		// new handler
		h := func(ctx context.Context, msgOutput *MessageOutput, err error) error {

			// validate messages exist, if no messages exist, do
			// not pass down the output and return nil
			if err == nil && len(msgOutput.Messages) == 0 || msgOutput.Messages == nil  {
				return nil
			}

			return handler(ctx, msgOutput, err)
		}

		return h
	}

	return f
}


