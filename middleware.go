package sqspoller

// Middleware is a function which that wraps a Handler
// to add functionality before or after the Handler code.
type Middleware func(Handler) Handler

// wrapMiddleware creates a new handler by wrapping middleware around a final
// handler. The middlewares' Handlers will be executed by requests in the order
// they are provided.
func wrapMiddleware(mw []Middleware, handler Handler) Handler {

	// Loop backwards through the middleware invoking each one. Replace the
	// handler with the new wrapped handler. Looping backwards ensures that the
	// first middleware of the slice is the first to be executed by requests.
	for i := len(mw) - 1; i >= 0; i-- {
		h := mw[i]
		if h != nil {
			handler = h(handler)
		}
	}

	return handler
}


// Use attaches global middleware to the Poller instance which will
// wrap any Handler and Handler specific middleware.
func (p *Poller) Use(middleware ...Middleware) {
	if p.middleware == nil {
		p.middleware = middleware
	} else {
		p.middleware = append(p.middleware, middleware...)
	}

}