package sqspoller

import (
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/sqs"
	"time"
)

// ReceiveMessageParams accepts the same parameters as the SQS ReceiveMessage
// method. It configures how the poller receives new messages, the parameters
// must be set before the Poller is run.
func (p *Poller) ReceiveMessageParams(input *sqs.ReceiveMessageInput, opts ...request.Option) {
	p.queueURL = *input.QueueUrl
	p.receiveMsgInput = input
	p.options = opts
}

// SetIdlePollInterval sets the polling interval for when the queue is empty
// and poll requests are returning empty responses, leaving the handler idle.
//
// This interval will be reached through an exponential back off starting at 1
// second from when the first empty response is received after a non-empty
// response, consecutive empty responses will cause the interval to double each
// time until the set interval is reached. Once a successful response is returned,
// the interval drops back down to 0.
func (p *Poller) SetIdlePollInterval(t time.Duration) {
	p.IdlePollInterval = t
}

// SetHandlerTimeout lets the user set the timeout for handling a message, if
// the messageHandler function cannot finish execution within this time frame,
// the MessageHandler will return ErrHandlerTimeout. The error can be caught and
// handled by custom middleware, so the  user can choose to move onto the next
// poll request if they so wish.
func (p *Poller) SetHandlerTimeout(t time.Duration) {
	p.handlerTimeout = t
}

// SetRequestTimeout lets the user set the timeout on requesting for a new message
// from the SQS queue. If the timeout occurs, ErrRequestTimeout will be passed to
// the OnError handler. If the caller wishes to continue polling after a the timeout,
// the ErrRequestTimeout error must be whitelisted in the error handler.
func (p *Poller) SetRequestTimeout(t time.Duration) {
	p.RequestTimeout = t
}
