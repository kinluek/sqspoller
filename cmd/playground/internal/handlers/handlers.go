package handlers

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/kinluek/sqspoller"
)

// MessageHandler set up for poller configured to received one message at a time.
func MessageHandler(ctx context.Context, client *sqs.SQS, msgOutput *sqspoller.MessageOutput) error {
	msg := msgOutput.Messages[0]

	// do work on message
	fmt.Printf("GOT MESSAGE: %v\n", msg)

	// delete message from queue
	if _, err := msg.Delete(); err != nil {
		return err
	}
	return nil
}

// ErrorHandler set up to log AWS error details.
func ErrorHandler(ctx context.Context, err error) error {
	if awsErr, ok := err.(awserr.Error); ok {
		fmt.Println("CODE:", awsErr.Code())
		fmt.Println("ORIGINAL ERROR:", awsErr.OrigErr())
		fmt.Println("MESSAGE:", awsErr.Error())
	}
	return err
}
