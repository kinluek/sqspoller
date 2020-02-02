package setup

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/kinluek/sqspoller/internal/testing/setup/docker"
	"os"
	"testing"
	"time"
)

// SQS will setup the SQS container and return when it is ready to be interacted
// with. It should be passed a value to specify how many times the function should
// attempt to create the SQS queue before failing, these attempts are retried every
// second from when the container starts.
// A teardown function is also returned which should be invoked once the caller is
// done with the SQS instance.
func SQS(t *testing.T, createQueueAttempts int) (sqsSvc *sqs.SQS, queueURL *string, teardown func()) {
	testEnv := os.Getenv("ENVIRONMENT")

	// Create containerized SQS
	container := docker.StartLocalStackContainer(t, map[string]string{
		"SERVICES":    "sqs",
		"DEBUG":       "1",
		"DATA_DIR":    "/tmp/localstack/data",
		"DOCKER_HOST": "unix:///var/run/docker.sock",
	})

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

	// Create SQS client using AWS SDK
	sess := session.Must(session.NewSession(&aws.Config{
		Credentials: credentials.AnonymousCredentials,
		Endpoint:    aws.String(endPoint),
		Region:      aws.String("eu-west-1")},
	))
	svc := sqs.New(sess)

	// Create SQS queue in local container
	// Keep retrying as local AWS environment will take time to be ready.
	queueName := "test-queue"
	var qURL *string


	for i := 0; i < createQueueAttempts; i++ {
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
		t.Fatalf("failed to create queue in under %v seconds", createQueueAttempts)
	}

	teardown = func() {
		docker.StopContainer(t, container, 30*time.Second)
	}

	return svc, qURL, teardown
}

