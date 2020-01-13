package sqspoller

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


//func Context() Middleware {
//	f := func(handler Handler) Handler {
//		h := func(ctx context.Context, msg *MessageOutput, err error) error {
//			select {
//			case <-ctx.Done():
//				return ctx.Err()
//			default:
//				if err := handler(ctx, msg, err); err != nil {
//					return err
//				}
//				return nil
//			}
//		}
//		return h
//	}
//	return f
//}
