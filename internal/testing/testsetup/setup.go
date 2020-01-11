package testsetup

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"testing"
	"time"
)

// SQS will set up an SQS queue and return an initialised SQS client that can
// interact with it.
func SQS(t *testing.T, endPoint string, createQueueAttempts int) (sqsSvc *sqs.SQS, queueURL string, teardown func()) {
	t.Helper()

	// Create SQS client using AWS SDK
	sess := session.Must(session.NewSession(&aws.Config{
		Credentials: credentials.AnonymousCredentials,
		Endpoint:    aws.String(endPoint),
		Region:      aws.String("eu-west-1")},
	))
	svc := sqs.New(sess)

	// Create SQS queue in local container.
	// Keep retrying as local AWS environment will take time to be ready, if it is being
	// spun up from docker images.
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
		fmt.Println("[test-teardown] removing sqs resources...")
	}

	return svc, *qURL, teardown
}
