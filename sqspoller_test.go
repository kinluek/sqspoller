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

	t.Run("basic polling", func(t *testing.T) {
		t.Parallel()

		svc, queueURL, teardown := setupSQS(t)
		defer teardown()

		// ==============================================================
		// Send message to SQS queue.

		messageBody := "message-body"

		sendResp, err := svc.SendMessage(&sqs.SendMessageInput{
			QueueUrl:    queueURL,
			MessageBody: aws.String(messageBody),
		})
		if err != nil {
			t.Fatalf("failed to send message to SQS: %v", err)
		}

		// ==============================================================
		// Create new poller using local queue.

		poller := sqspoller.New(svc, sqs.ReceiveMessageInput{
			QueueUrl: queueURL,
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
				return confirmedRunning
			}
			return nil
		}

		poller.Handle(handler)
		err = poller.StartPolling()

		pollErr := err.(*sqspoller.Error)
		if pollErr.OriginalError != confirmedRunning {
			t.Fatalf("could not run poller: %v", err)
		}
	})

	t.Run("timeout no messages received", func(t *testing.T) {
		t.Parallel()

		svc, queueURL, teardown := setupSQS(t)
		defer teardown()

		// ==============================================================
		// Create new poller and configure Timeouts

		poller := sqspoller.New(svc, sqs.ReceiveMessageInput{
			QueueUrl: queueURL,
		})

		poller.AllowTimeout = true
		poller.TimeoutNoMessages = 3 * time.Second

		// ==============================================================
		// Start Polling with no messages to be received

		handler := func(ctx context.Context, msgOut *sqspoller.MessageOutput, err error) error {
			return nil
		}

		poller.Handle(handler)
		err := poller.StartPolling()

		pollErr := err.(*sqspoller.Error)
		if pollErr.OriginalError != sqspoller.ErrTimeoutNoMessages {
			t.Fatalf("could not run poller: %v", pollErr)
		}
	})

	t.Run("timeout after receiving a few messages, then none after", func(t *testing.T) {
		t.Parallel()

		svc, queueURL, teardown := setupSQS(t)
		defer teardown()

		// ==============================================================
		// Create new poller and configure Timeouts

		poller := sqspoller.New(svc, sqs.ReceiveMessageInput{
			QueueUrl: queueURL,
		})

		poller.AllowTimeout = true
		poller.TimeoutNoMessages = 3 * time.Second
		poller.Interval = time.Microsecond

		// ==============================================================
		// Start Polling with messages arriving every second for 4 Seconds
		go func() {
			for i := 0; i < 4; i++ {
				_, err := svc.SendMessage(&sqs.SendMessageInput{
					QueueUrl:    queueURL,
					MessageBody: aws.String(strconv.Itoa(i)),
				})
				if err != nil {
					t.Fatalf("failed to send message to SQS: %v", err)
				}
				time.Sleep(time.Second)
			}
		}()

		var messagesReceived int64

		handler := func(ctx context.Context, msgOut *sqspoller.MessageOutput, err error) error {
			if len(msgOut.Messages) > 0 {
				atomic.AddInt64(&messagesReceived, 1)
				_, err :=  msgOut.Messages[0].Delete()
				return err
			}
			return nil
		}

		poller.Handle(handler)
		err := poller.StartPolling()

		pollErr := err.(*sqspoller.Error)
		if pollErr.OriginalError != sqspoller.ErrTimeoutNoMessages {
			t.Fatalf("could not run poller: %v", pollErr)
		}

		if messagesReceived != 4 {
			t.Fatalf("expected to receive %v messages, got %v", 4, messagesReceived)
		}
	})
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