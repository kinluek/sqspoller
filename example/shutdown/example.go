package shutdown

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/kinluek/sqspoller"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func Handler(ctx context.Context, client *sqs.SQS, msgOutput *sqspoller.MessageOutput, err error) error {
	// check errors returned from polling the queue.
	if err != nil {
		return err
	}
	msg := msgOutput.Messages[0]

	// do work on message
	fmt.Println("GOT MESSAGE: ", msg)

	// delete message from queue
	if _, err := msg.Delete(); err != nil {
		return err
	}
	return nil
}

func main() {
	sess := session.Must(session.NewSession())
	sqsClient := sqs.New(sess)

	poller := sqspoller.Default(sqsClient)

	poller.ReceiveMessageParams(&sqs.ReceiveMessageInput{
		MaxNumberOfMessages: aws.Int64(1),
		QueueUrl:            aws.String("https://sqs.us-east-2.amazonaws.com/123456789012/MyQueue"),
	})

	poller.Handle(Handler)

	// run poller in a separate goroutine and wait for errors on channel
	pollerErrors := make(chan error, 1)
	go func() {
		pollerErrors <- poller.Run()
	}()

	// listen for shutdown signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-pollerErrors:
		log.Fatal(err)
	case <-shutdown:
		if err := poller.ShutdownAfter(30 * time.Second); err != nil {
			log.Fatal(err)
		}
	}
}
