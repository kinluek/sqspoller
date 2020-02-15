package sqspoller

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
)

// MessageOutput is contains the SQS ReceiveMessageOutput and is passed down to
// the MessageHandler when the Poller is running.
type MessageOutput struct {
	*sqs.ReceiveMessageOutput
	Messages []*Message
	queueURL string
}

// Message is an individual message, contained within a MessageOutput, it provides
// methods to remove itself from the SQS queue.
type Message struct {
	*sqs.Message

	client   *sqs.SQS
	queueURL string
}

// Delete removes the message from the queue, permanently.
func (m *Message) Delete() (*sqs.DeleteMessageOutput, error) {
	return m.client.DeleteMessage(&sqs.DeleteMessageInput{
		QueueUrl:      aws.String(m.queueURL),
		ReceiptHandle: m.ReceiptHandle,
	})
}

// convertMessage converts an sqs.ReceiveMessageOutput to sqspoller.MessageOutput.
func convertMessage(msgOut *sqs.ReceiveMessageOutput, svc *sqs.SQS, qURL string) *MessageOutput {
	var messages []*Message
	if !messageOutputIsEmpty(msgOut) {
		messages = make([]*Message, len(msgOut.Messages))
		for i, msg := range msgOut.Messages {
			messages[i] = &Message{
				Message:  msg,
				client:   svc,
				queueURL: qURL,
			}
		}
	}
	return &MessageOutput{
		ReceiveMessageOutput: msgOut,
		Messages:             messages,
		queueURL:             qURL,
	}
}

// messageOutputIsEmpty takes an sqs.ReceiveMessageOutput and returns true if the
// output is empty.
func messageOutputIsEmpty(out *sqs.ReceiveMessageOutput) bool {
	if out.Messages == nil || len(out.Messages) == 0 {
		return true
	}
	return false
}
