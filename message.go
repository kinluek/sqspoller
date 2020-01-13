package sqspoller

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
)

// MessageOutput is contains the SQS ReceiveMessageOutput and
// is passed down to the Handler when the Poller is running.
type MessageOutput struct {
	*sqs.ReceiveMessageOutput
	Messages []*Message

	client   *sqs.SQS
	queueURL string
}

// Message is an individual message, contained within
// a MessageOutput, it provides methods to remove
// itself from the SQS queue.
type Message struct {
	*sqs.Message

	client   *sqs.SQS
	queueURL string
}

// messageOutput transforms an sqs.ReceiveMessageOutput to
// sqspoller.MessageOutput.
func messageOutput(msgOut *sqs.ReceiveMessageOutput, svc *sqs.SQS, qURL string) *MessageOutput {
	messages := make([]*Message, 0)
	for _, msg := range msgOut.Messages {
		message := Message{
			Message:  msg,
			client:   svc,
			queueURL: qURL,
		}
		messages = append(messages, &message)
	}
	return &MessageOutput{
		ReceiveMessageOutput: msgOut,
		Messages:             messages,
		client:               svc,
		queueURL:             qURL,
	}
}

// Delete removes the message from the queue, permanently.
func (m *Message) Delete() (*sqs.DeleteMessageOutput, error) {
	return m.client.DeleteMessage(&sqs.DeleteMessageInput{
		QueueUrl:      aws.String(m.queueURL),
		ReceiptHandle: m.ReceiptHandle,
	})
}
