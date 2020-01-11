package sqspoller_test

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/kinluek/sqspoller"
	"testing"
)

func TestPoller_Handle(t *testing.T) {
	p := &sqspoller.Poller{}

	p.Handle(func(ctx context.Context, msg *sqs.ReceiveMessageOutput, err error) error {
		return nil
	})
}

func TestPoller_EndToEnd(t *testing.T) {
	sess := session.Must(session.NewSession(&aws.Config{
		Credentials: credentials.AnonymousCredentials,
		Endpoint:    aws.String("http://localhost:4576"),
		Region:      aws.String("eu-west-1")},
	))

	svc := sqs.New(sess)

	poller := sqspoller.New(svc, sqs.ReceiveMessageInput{
		QueueUrl: aws.String("http://localhost:4576/queue/test-queue"),
	})

	poller.Handle(func(ctx context.Context, msg *sqs.ReceiveMessageOutput, err error) error {
		if err != nil {
			return err
		}

		fmt.Println(msg)
		return nil
	})

	if err := poller.Run(); err != nil {
		t.Fatalf("could not run poller: %v", err)
	}
}
