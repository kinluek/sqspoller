// The Playground is where you can run the poller locally against a containerized
// SQS service.
//
// Run 'go run main.go' to see the poller in action. Experiment with the poller by
// sending messages from the command line, altering the code or changing the configuration
// variables.
//
// Run 'go run main.go --help' to see what configurations variables are available.
//
// Before running the playground poller, make sure docker is installed and running
// on your machine.
package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/kinluek/sqspoller"
	"github.com/kinluek/sqspoller/cmd/playground/internal/setup"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	region    = "eu-west-1"
	queueName = "test-queue"
)

var (
	it = flag.Int("idle-poll-interval", 4, "sets the interval time in seconds between each poll when queue is empty")
	st = flag.Int("shutdown-timeout", 5, "sets the shutdown timeout in seconds")
)

// MessageHandler set up for poller configured to received one message at a time.
func MessageHandler(ctx context.Context, client *sqs.SQS, msgOutput *sqspoller.MessageOutput) error {

	// access tracking values from context
	v := ctx.Value(sqspoller.TrackingKey).(*sqspoller.TrackingValue)
	fmt.Println("Trace ID: ", v.TraceID)
	fmt.Println("Receive Time: ", v.Now)

	// do work on message
	msg := msgOutput.Messages[0]
	fmt.Println("GOT MESSAGE: ", msg)

	// delete message from queue
	if _, err := msg.Delete(); err != nil {
		return err
	}
	return nil
}

// ErrorHandler set up to log AWS error details.
func ErrorHandler(ctx context.Context, err error) error {

	// access tracking values from context
	v := ctx.Value(sqspoller.TrackingKey).(*sqspoller.TrackingValue)
	fmt.Println("Trace ID: ", v.TraceID)
	fmt.Println("Receive Time: ", v.Now)

	if awsErr, ok := err.(awserr.Error); ok {
		fmt.Println("CODE:", awsErr.Code())
		fmt.Println("ORIGINAL ERROR:", awsErr.OrigErr())
		fmt.Println("MESSAGE:", awsErr.Error())
	}
	return err
}

func run() (err error) {

	log := log.New(os.Stdout, "[playground] ", 0)

	//==============================================================
	// Parse playground args
	flag.Parse()
	idlePollInterval := time.Duration(*it) * time.Second
	shutdownTimeout := time.Duration(*st) * time.Second

	log.Println("[args] --idle-poll-interval:", idlePollInterval)
	log.Println("[args] --shutdown-timeout:", shutdownTimeout)

	//==============================================================
	// Setting up localstack SQS
	log.Println("[docker] setting up localstack...")

	env, teardown, err := setup.NewEnv(region, queueName)
	if err != nil {
		return fmt.Errorf("[docker] could not set up environment: %v", err)
	}
	defer teardown()

	//==============================================================
	// Listen for text input to send to SQS
	go setup.QueueMessageInput(log, env.Client, env.Queue)

	//==============================================================
	// Starting Poller
	log.Println("[poller] starting poller...")

	poller := sqspoller.Default(env.Client)
	poller.ReceiveMessageParams(&sqs.ReceiveMessageInput{
		MaxNumberOfMessages: aws.Int64(1),
		QueueUrl:            env.Queue,
	})
	poller.SetIdlePollInterval(idlePollInterval)
	poller.OnMessage(MessageHandler)
	poller.OnError(ErrorHandler)

	pollerErrors := make(chan error, 1)
	go func() {
		pollerErrors <- poller.Run()
	}()

	//==============================================================
	// Handle Shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-pollerErrors:
		return fmt.Errorf("[poller] encountered polling error: %v", err)
	case <-shutdown:
		log.Println("[poller] shutdown signal received")
		if err := poller.ShutdownAfter(shutdownTimeout); err != nil {
			return fmt.Errorf("[poller] shutting down: %v", err)
		}
		log.Printf("[poller] shutdown finished")
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
