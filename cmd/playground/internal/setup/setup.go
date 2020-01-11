package setup

import (
	"bufio"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/kinluek/sqspoller/internal/testing/docker"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// SQS contains the SQS resources needed to set up a Poller.
type SQS struct {
	Client *sqs.SQS
	Queue  *string
}

// NewEnv will set up the a new environment for the playground, a local SQS instance
// will be returned when it is ready to be interacted with. A teardown function is also
// returned, which should be executed once the caller is done with the SQS instance.
func NewEnv(region, queueName string) (env *SQS, teardown func() error, err error) {

	// ==============================================================
	// Setup localstack with SQS
	container, err := docker.StartLocalStackContainer(map[string]string{
		"SERVICES":    "sqs",
		"DEBUG":       "1",
		"DATA_DIR":    "/tmp/localstack/data",
		"DOCKER_HOST": "unix:///var/run/docker.sock",
	})
	if err != nil {
		return nil, nil, err
	}

	sqsEndpoint := "http://localhost:" + container.ExposedPorts["4576"][0].HostPort

	// ==============================================================
	// Wait for SQS to be ready and create queue.
	sess := session.Must(session.NewSession(&aws.Config{
		Credentials: credentials.AnonymousCredentials,
		Endpoint:    aws.String(sqsEndpoint),
		Region:      aws.String(region),
	}))
	sqsClient := sqs.New(sess)

	var qURL *string
	var createErr error

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		limitSecs := 30
		for i := 0; i < limitSecs; i++ {
			result, err := sqsClient.CreateQueue(&sqs.CreateQueueInput{
				QueueName: aws.String(queueName),
			})
			if err == nil {
				qURL = result.QueueUrl
				return
			}
			time.Sleep(time.Second)
		}
		createErr = fmt.Errorf("failed to create queue in under %v seconds", limitSecs)
	}()

	wg.Wait()
	if createErr != nil {
		return nil, nil, createErr
	}

	e := SQS{
		Client: sqsClient,
		Queue:  qURL,
	}

	teardown = func() error {
		fmt.Println("cleaning up container resources...")
		return docker.StopContainer(container, 30*time.Second)
	}

	return &e, teardown, nil
}

// QueueMessageInput listens to text on stdin and sends it to the SQS queue
// for the poller to receive.
func QueueMessageInput(log *log.Logger, client *sqs.SQS, queueURL *string) {
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
