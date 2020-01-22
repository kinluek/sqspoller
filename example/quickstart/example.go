package main

import (
	"context"
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

	// use client to create default Poller instance.
	poller := sqspoller.Default(sqsClient)

	// supply polling parameters.
	poller.ReceiveMessageParams(&sqs.ReceiveMessageInput{
		MaxNumberOfMessages: aws.Int64(1),
		QueueUrl:            aws.String("https://sqs.us-east-2.amazonaws.com/123456789012/MyQueue"),
	})

	// configure poll interval and handler timeout
	poller.SetPollInterval(30 * time.Second)
	poller.SetHandlerTimeout(120 * time.Second)

	// supply handler to handle new messages
	poller.OnMessage(func(ctx context.Context, client *sqs.SQS, msgOutput *sqspoller.MessageOutput) error {
		msg := msgOutput.Messages[0]
		// do work on message
		fmt.Println("GOT MESSAGE: ", msg)
		// delete message from queue
		if _, err := msg.Delete(); err != nil {
			return err
		}
		return nil
	})

	// supply handler to handle errors returned from poll requests to
	// SQS returning a non nil error will cause the poller to exit.
	poller.OnError(func(ctx context.Context, err error) error {
		// log error and exit poller.
		log.Println(err)
		return err
	})

	// Run poller.
	if err := poller.Run(); err != nil {
		log.Fatal(err)
	}
}
