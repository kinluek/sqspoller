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
	"testing"
	"time"
)


func TestPoller(t *testing.T) {
	testEnv := os.Getenv("ENVIRONMENT")

	// ==============================================================
	// Setup local containerized SQS

	container := docker.StartLocalStackContainer(t, map[string]string{
		"SERVICES":    "sqs",
		"DEBUG":       "1",
		"DATA_DIR":    "/tmp/localstack/data",
		"DOCKER_HOST": "unix:///var/run/docker.sock",
	}, os.Getenv("TMPDIR"))
	defer container.Cleanup()

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
	var queueURL *string

	limitSecs := 20
	for i := 0; i < limitSecs; i++ {
		result, err := svc.CreateQueue(&sqs.CreateQueueInput{
			QueueName: aws.String(queueName),
		})
		if err == nil {
			t.Log("queue created: ", *result.QueueUrl)
			queueURL = result.QueueUrl
			break
		}
		time.Sleep(time.Second)
	}
	if queueURL == nil {
		t.Fatalf("failed to create queue in under %v seconds", limitSecs)
	}

	messageBody := "message-body"

	sendResp, err := svc.SendMessage(&sqs.SendMessageInput{
		QueueUrl:    queueURL,
		MessageBody: aws.String(messageBody),
	})

	// ==============================================================
	// Create new poller using local queue

	poller := sqspoller.New(svc, sqs.ReceiveMessageInput{
		QueueUrl: queueURL,
	})

	// ==============================================================
	// Attach Handler and Start Polling.
	// Assert that the correct message is received and that the correct
	// error is returned.

	confirmedRunning := errors.New("started and exited")

	handler := func(ctx context.Context, msg *sqspoller.Message, err error) error {
		if *msg.Messages[0].Body != messageBody {
			t.Fatalf("received message body: %v, wanted: %v", *msg.Messages[0].Body, messageBody)
		}
		if *msg.Messages[0].MessageId != *sendResp.MessageId {
			t.Fatalf("received message ID: %v, wanted: %v", *msg.Messages[0].MessageId, *sendResp.MessageId)
		}
		return confirmedRunning
	}

	poller.Handle(handler)
	err = poller.StartPolling()

	pollErr := err.(*sqspoller.Error)
	if pollErr.OriginalError != confirmedRunning {
		t.Fatalf("could not run poller: %v", err)
	}
}
