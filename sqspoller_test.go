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

	// ==============================================================
	// Setup local containerized SQS

	container := docker.StartLocalStackContainer(t, map[string]string{
		"SERVICES":    "sqs",
		"DEBUG":       "1",
		"DATA_DIR":    "/tmp/localstack/data",
		"DOCKER_HOST": "unix:///var/run/docker.sock",
	}, os.Getenv("TMPDIR"))
	defer container.Cleanup()

	sqsHostPort := container.ExposedPorts["4576"][0].HostPort
	endPoint := "http://localhost:"+sqsHostPort


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

	for i := 0; i < 20; i++ {
		result, err := svc.CreateQueue(&sqs.CreateQueueInput{
			QueueName: aws.String(queueName),
		})
		if err == nil {
			t.Log("queue created: ", *result.QueueUrl)
			queueURL = result.QueueUrl
			break
		}
		t.Log("waiting for container to be ready...")
		time.Sleep(time.Second)
	}

	// ==============================================================
	// Create new poller using local queue

	poller := sqspoller.New(svc, sqs.ReceiveMessageInput{
		QueueUrl: queueURL,
	})

	// ==============================================================
	// Attach Handler that a known value

	confirmedRunning := errors.New("started and exited")
	handler := func(ctx context.Context, msg *sqs.ReceiveMessageOutput, err error) error {
		return confirmedRunning
	}

	// ==============================================================
	// Run Poller and assert that the known value is returned
	// to confirm that the poller runs fine.

	poller.Handle(handler)
	err := poller.Run()

	pollErr := err.(*sqspoller.Error)
	if pollErr.OriginalError != confirmedRunning {
		t.Fatalf("could not run poller: %v", err)
	}
}
