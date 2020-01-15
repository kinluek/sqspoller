package sqspoller_test

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/kinluek/sqspoller"
	"github.com/kinluek/sqspoller/internal/testing/docker"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestPoller(t *testing.T) {
	svc, queueURL, teardown := setupSQS(t)
	defer teardown()

	tests := PollerTests{svc, queueURL}

	t.Run("basic polling", tests.BasicPolling)
	t.Run("timeout - no messages to receive", tests.TimeoutNoMessages)
	t.Run("timeout - after several messages", tests.TimeoutAfterSeveralMessages)
	t.Run("timeout - reset if timeout happens during message handling", tests.TimeoutResetIfHandlingMessage)
}

// PollerTests holds the tests for the Poller
type PollerTests struct {
	sqsClient *sqs.SQS
	queueURL  *string
}

func (p *PollerTests) BasicPolling(t *testing.T) {
	// ==============================================================
	// Send message to SQS queue.
	messageBody := "message-body"

	sendResp, err := p.sqsClient.SendMessage(&sqs.SendMessageInput{
		QueueUrl:    p.queueURL,
		MessageBody: aws.String(messageBody),
	})
	if err != nil {
		t.Fatalf("failed to send message to SQS: %v", err)
	}

	// ==============================================================
	// Create new poller using local queue.

	poller := sqspoller.New(p.sqsClient, sqs.ReceiveMessageInput{
		QueueUrl: p.queueURL,
	})

	// ==============================================================
	// Attach Handler and Start Polling.
	// Assert that the correct message is received and that the correct
	// error is returned.

	confirmedRunning := errors.New("started and exited")

	handler := func(ctx context.Context, msgOut *sqspoller.MessageOutput, err error) error {
		if len(msgOut.Messages) > 0 {
			if *msgOut.Messages[0].Body != messageBody {
				t.Fatalf("received message body: %v, wanted: %v", *msgOut.Messages[0].Body, messageBody)
			}
			if *msgOut.Messages[0].MessageId != *sendResp.MessageId {
				t.Fatalf("received message ID: %v, wanted: %v", *msgOut.Messages[0].MessageId, *sendResp.MessageId)
			}
			if _, err := msgOut.Messages[0].Delete(); err != nil {
				return err
			}
			return confirmedRunning
		}
		return confirmedRunning
	}

	poller.Handle(handler)

	err = poller.StartPolling()
	if err != confirmedRunning {
		t.Fatalf("could not run poller: %v", err)
	}
}

func (p *PollerTests) TimeoutNoMessages(t *testing.T) {
	// ==============================================================
	// Create new poller and configure Timeouts

	poller := sqspoller.New(p.sqsClient, sqs.ReceiveMessageInput{
		QueueUrl: p.queueURL,
	})

	poller.ExitAfterNoMessagesReceivedFor(time.Millisecond)

	// ==============================================================
	// Start Polling with no messages to be received

	handler := func(ctx context.Context, msgOut *sqspoller.MessageOutput, err error) error {
		return nil
	}

	poller.Handle(handler)

	err := poller.StartPolling()
	if err != sqspoller.ErrTimeoutNoMessages {
		t.Fatalf("could not run poller: %v", err)
	}
}

func (p *PollerTests) TimeoutAfterSeveralMessages(t *testing.T) {
	// ==============================================================
	// Create new poller and configure Timeouts

	poller := sqspoller.New(p.sqsClient, sqs.ReceiveMessageInput{
		QueueUrl:        p.queueURL,
		WaitTimeSeconds: aws.Int64(2),
	})

	poller.ExitAfterNoMessagesReceivedFor(1 * time.Second)
	poller.SetInterval(0)

	// ==============================================================
	// Start Polling with messages arriving every second for 4 Seconds
	go func() {
		for i := 0; i < 4; i++ {
			_, err := p.sqsClient.SendMessage(&sqs.SendMessageInput{
				QueueUrl:    p.queueURL,
				MessageBody: aws.String(strconv.Itoa(i)),
			})
			if err != nil {
				t.Fatalf("failed to send message to SQS: %v", err)
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()

	var messagesReceived int64

	handler := func(ctx context.Context, msgOut *sqspoller.MessageOutput, err error) error {
		if len(msgOut.Messages) > 0 {
			atomic.AddInt64(&messagesReceived, 1)
			_, err := msgOut.Messages[0].Delete()
			if err != nil {
				t.Fatalf("could not delete message: %v", err)
			}
			return nil
		}
		return nil
	}

	poller.Handle(handler)
	err := poller.StartPolling()

	if err != sqspoller.ErrTimeoutNoMessages {
		t.Fatalf("could not run poller: %v", err)
	}

	if messagesReceived != 4 {
		t.Fatalf("expected to receive %v messages, got %v", 4, messagesReceived)
	}
}

func (p *PollerTests) TimeoutResetIfHandlingMessage(t *testing.T) {
	// ==============================================================
	// Create new poller and configure Timeouts

	poller := sqspoller.New(p.sqsClient, sqs.ReceiveMessageInput{
		QueueUrl:        p.queueURL,
		WaitTimeSeconds: aws.Int64(2),
	})

	timeout := time.Second
	poller.ExitAfterNoMessagesReceivedFor(timeout)
	poller.SetInterval(0)

	// ==============================================================
	// Pre-fill Queue with a single message

	_, err := p.sqsClient.SendMessage(&sqs.SendMessageInput{
		QueueUrl:    p.queueURL,
		MessageBody: aws.String("message"),
	})
	if err != nil {
		t.Fatalf("failed to send message to SQS: %v", err)
	}

	var messagesReceived int

	confirmed := errors.New("confirmed poller did not timeout after first message")

	handler := func(ctx context.Context, msgOut *sqspoller.MessageOutput, err error) error {
		if messagesReceived == 1 {
			return confirmed
		}

		if len(msgOut.Messages) > 0 {
			time.Sleep(2*timeout)

			messagesReceived++
			_, err := msgOut.Messages[0].Delete()
			if err != nil {
				t.Fatalf("could not delete message: %v", err)
			}
			return nil
		}

		return nil
	}

	poller.Handle(handler)
	err = poller.StartPolling()

	if err != confirmed {
		t.Fatalf("poller errored out: %v", err)
	}

	if messagesReceived != 1 {
		t.Fatalf("expected to receive %v messages, got %v", 4, messagesReceived)
	}
}

// setupSQS will setup the SQS container and return when it is ready
// to be interacted with. A teardown function is also returned and it's
// execution should be deferred.
func setupSQS(t *testing.T) (sqsSvc *sqs.SQS, queueURL *string, teardown func()) {
	testEnv := os.Getenv("ENVIRONMENT")

	// ==============================================================
	// Setup local containerized SQS

	container := docker.StartLocalStackContainer(t, map[string]string{
		"SERVICES":    "sqs",
		"DEBUG":       "1",
		"DATA_DIR":    "/tmp/localstack/data",
		"DOCKER_HOST": "unix:///var/run/docker.sock",
	}, os.Getenv("TMPDIR"))

	endPoint := "http://localhost:" + container.ExposedPorts["4576"][0].HostPort

	if testEnv == "CI" {

		// if the container has been started in a CI environment
		// then localstack will run as a sibling container, therefore,
		// we need to connect it to the same docker network as the
		// application container to interact with it.
		docker.NetworkConnect(t, os.Getenv("DOCKER_NETWORK"), container.ID)

		// first 12 characters of the container ID will be used
		// as an alias when adding to the network.
		endPoint = "http://" + container.ID[:12] + ":" + "4576"
	}

	// ==============================================================
	// Setup SQS client using AWS SDK

	sess := session.Must(session.NewSession(&aws.Config{
		Credentials: credentials.AnonymousCredentials,
		Endpoint:    aws.String(endPoint),
		Region:      aws.String("eu-west-1")},
	))
	svc := sqs.New(sess)

	// ==============================================================
	// Create SQS queue in local container
	// Keep retrying as local AWS environment will take time to be ready.

	queueName := "test-queue"
	var qURL *string

	limitSecs := 30
	for i := 0; i < limitSecs; i++ {
		result, err := svc.CreateQueue(&sqs.CreateQueueInput{
			QueueName: aws.String(queueName),
		})
		if err == nil {
			qURL = result.QueueUrl
			break
		}
		time.Sleep(time.Second)
	}
	if qURL == nil {
		t.Fatalf("failed to create queue in under %v seconds", limitSecs)
	}

	return svc, qURL, container.Cleanup
}
