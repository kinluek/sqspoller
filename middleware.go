package sqspoller

import (
	"context"
	"github.com/aws/aws-sdk-go/service/sqs"
	"time"
)

// Middleware is a function which that wraps a MessageHandler to add functionality
// before or after the MessageHandler code.
type Middleware func(MessageHandler) MessageHandler

// Use attaches global outerMiddleware to the Poller instance which will wrap any
// MessageHandler and MessageHandler specific outerMiddleware.
func (p *Poller) Use(middleware ...Middleware) {
	p.mu.Lock()
	{
		if p.outerMiddleware == nil {
			p.outerMiddleware = middleware
		} else {
			p.outerMiddleware = append(p.outerMiddleware, middleware...)
		}
	}
	p.mu.Unlock()
}

// wrapMiddleware creates a new message handler by wrapping the middleware around
// the given handler. The middlewares will be executed  in the order they are provided.
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

// applyTimeout applies a timeout to the message handler if the timeout is greater
// than 0. If timeout is 0, then the function returns the message handler unchanged.
func applyTimeout(handler MessageHandler, timeout time.Duration) MessageHandler {
	if timeout > 0 {
		handler = wrapMiddleware(handler, handlerTimeout(timeout))
	}
	return handler
}

// handlerTimeout takes a timeout duration and returns ErrHandlerTimeout if the
// message handler cannot process the message within that time. The user can then
// use other middleware to check for ErrHandlerTimeout and decide whether to
// exit or move onto the next poll request.
//
// handlerTimeout middleware can only be applied via the Poller.SetHandlerTimeout
// method.
func handlerTimeout(t time.Duration) Middleware {

	f := func(handler MessageHandler) MessageHandler {

		h := func(ctx context.Context, client *sqs.SQS, msgOut *MessageOutput) error {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

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

// IgnoreEmptyResponses stops the data from being passed down to the inner message
// handler, if there is no message to be handled.
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
