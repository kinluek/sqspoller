package sqspoller

import (
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/sqs"
	"time"
)

// ReceiveMessageParams accepts the same parameters as the SQS
// ReceiveMessage method. It configures how the poller receives
// new messages, the parameters must be set before the Poller is
// run.
func (p *Poller) ReceiveMessageParams(input *sqs.ReceiveMessageInput, opts ...request.Option) {
	p.queueURL = *input.QueueUrl
	p.receiveMsgInput = input
	p.options = opts
}

// SetPollInterval lets the user set the time interval between
// poll requests.
func (p *Poller) SetPollInterval(t time.Duration) {
	p.PollInterval = t
}

// SetHandlerTimeout lets the user set the timeout for handling
// a message, if the handler function cannot finish execution
// within this time frame, the Handler will return ErrHandlerTimeout.
// The error can be caught and handled by custom middleware, so
// the  user can choose to move onto the next poll request if they
// so wish.
func (p *Poller) SetHandlerTimeout(t time.Duration) {
	p.HandlerTimeout = t
}
