package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/kinluek/sqspoller"
	"log"
	"time"
)

func main() {

	// create SQS client.
	sess := session.Must(session.NewSession())
	sqsClient := sqs.New(sess)

	// use client to create Poller instance.
	poller := sqspoller.New(sqsClient)

	// IgnoreEmptyResponses stops empty message outputs from reaching the core handler
	// and therefore the user can guarantee that there will be at least one message in
	// the message output.
	poller.Use(sqspoller.IgnoreEmptyResponses())

	// Tracking adds tracking values to the context object which can be retrieved using
	// sqspoller.CtxKey.
	poller.Use(sqspoller.Tracking())

	// supply polling parameters.
	poller.ReceiveMessageParams(&sqs.ReceiveMessageInput{
		MaxNumberOfMessages: aws.Int64(1),
		QueueUrl:            aws.String("https://sqs.us-east-2.amazonaws.com/123456789012/MyQueue"),
	})

	// configure poll interval and handler timeout
	poller.SetPollInterval(30 * time.Second)
	poller.SetHandlerTimeout(120 * time.Second)

	// supply handler to handle new messages
	poller.Handle(func(ctx context.Context, client *sqs.SQS, msgOutput *sqspoller.MessageOutput, err error) error {
		// check errors returned from polling the queue.
		if err != nil {
			return err
		}
		msg := msgOutput.Messages[0]

		// get tracking values provided by Tracking middleware.
		v, ok := ctx.Value(sqspoller.CtxKey).(*sqspoller.CtxTackingValue)
		if !ok {
			return errors.New("tracking middleware should have provided traced ID and receive time")
		}

		fmt.Println(v.TraceID, v.Now)

		// delete message from queue
		if _, err := msg.Delete(); err != nil {
			return err
		}
		return nil
	})

	if err := poller.Run(); err != nil {
		log.Fatal(err)
	}
}
