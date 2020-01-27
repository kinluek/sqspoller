package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/kinluek/sqspoller"
	"github.com/kinluek/sqspoller/cmd/playground/internal/handlers"
	"github.com/kinluek/sqspoller/cmd/playground/internal/setup"
	"log"
	"os"
	"os/signal"
	"strings"
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

func run() (err error) {

	pgLog := log.New(os.Stdout, "[playground] ", 0)

	//==============================================================
	// Parse playground args
	flag.Parse()
	idlePollInterval := time.Duration(*it) * time.Second
	shutdownTimeout := time.Duration(*st) * time.Second

	pgLog.Println("[args] --idle-poll-interval:", idlePollInterval)
	pgLog.Println("[args] --shutdown-timeout:", shutdownTimeout)

	//==============================================================
	// Setting up localstack SQS
	pgLog.Println("[docker] setting up localstack...")

	env, teardown, err := setup.Localstack(region, queueName)
	if err != nil {
		return fmt.Errorf("[docker] could not setup localstack: %v", err)
	}
	defer teardown()

	//==============================================================
	// Listen for text input to send to SQS
	go queueMessageInput(pgLog, env.Client, env.Queue)

	//==============================================================
	// Starting Poller
	pgLog.Println("[poller] starting poller...")

	poller := sqspoller.Default(env.Client)
	poller.ReceiveMessageParams(&sqs.ReceiveMessageInput{
		MaxNumberOfMessages: aws.Int64(1),
		QueueUrl:            env.Queue,
	})
	poller.SetIdlePollInterval(idlePollInterval)
	poller.OnMessage(handlers.MessageHandler)
	poller.OnError(handlers.ErrorHandler)

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
		pgLog.Println("[poller] shutdown signal received")
		if err := poller.ShutdownAfter(shutdownTimeout); err != nil {
			return fmt.Errorf("[poller] shutting down: %v", err)
		}
	}

	return nil
}

// queueMessageInput listens to text on stdin and sends it to the SQS queue
// for the poller to receive.
func queueMessageInput(log *log.Logger, client *sqs.SQS, queueURL *string) {
	log.Println("[input] enter text to standard in, then press enter to send the message:")
	reader := bufio.NewReader(os.Stdin)

	for {
		msg, _ := reader.ReadString('\n')
		msg = strings.TrimSpace(msg)

		_, err := client.SendMessage(&sqs.SendMessageInput{
			MessageBody: aws.String(msg),
			QueueUrl:    queueURL,
		})
		if err != nil {
			log.Printf("[input] could not send message: %v\n", err)
		} else {
			log.Println("[input] message sent")
		}
	}
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
