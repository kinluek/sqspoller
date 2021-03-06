[![kinluek](https://circleci.com/gh/kinluek/sqspoller.svg?style=shield)](https://circleci.com/gh/kinluek/sqspoller)
[![GoDoc](https://godoc.org/github.com/kinluek/sqspoller?status.svg)](https://godoc.org/github.com/kinluek/sqspoller)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

# SQS-Poller

SQS-Poller is a simple queue polling framework, designed specifically to work with AWS SQS.

## Contents

-   [Installation](#installation)
-   [Features](#features)
-   [Quick Start](#quick-start)
-   [Using Middleware](#using-middleware)
-   [Handling Shutdowns](#shutdown)
-   [The Playground](#the-playground)
-   [Go Version](#go-version)
-   [Testing](#testing)

## Installation

1. Install sqspoller:

```sh
$ go get -u github.com/kinluek/sqspoller
```

2. Import code:

```go
import "github.com/kinluek/sqspoller"
```

## Features

-   Timeouts
-   Polling intervals
-   Polling back offs on empty responses
-   Graceful shutdowns
-   Middleware
-   Remove message from queue with simple delete API

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/kinluek/sqspoller"
	"log"
	"time"
)

func main() {
	// create SQS client.
	sess := session.Must(session.NewSession())
	sqsClient := sqs.New(sess)

	// use client to create default Poller instance.
	poller := sqspoller.Default(sqsClient)

	// supply polling parameters.
	poller.ReceiveMessageParams(&sqs.ReceiveMessageInput{
		MaxNumberOfMessages: aws.Int64(1),
		QueueUrl:            aws.String("https://sqs.us-east-2.amazonaws.com/123456789012/MyQueue"),
	})

	// configure idle poll interval and handler timeout
	poller.SetIdlePollInterval(30 * time.Second)
	poller.SetHandlerTimeout(120 * time.Second)

	// supply handler to handle new messages
	poller.OnMessage(func(ctx context.Context, client *sqs.SQS, msgOutput *sqspoller.MessageOutput) error {
		msg := msgOutput.Messages[0]
		// do work on message
		fmt.Println("GOT MESSAGE: ", msg)
		// delete message from queue
		if _, err := msg.Delete(); err != nil {
			return err
		}
		return nil
	})

	// supply handler to handle errors returned from poll requests to
	// SQS returning a non nil error will cause the poller to exit.
	poller.OnError(func(ctx context.Context, err error) error {
		// log error and exit poller.
		log.Println(err)
		return err
	})

	// Run poller.
	if err := poller.Run(); err != nil {
		log.Fatal(err)
	}
}
```

## Using Middleware

```go
func main() {

	poller := sqspoller.New(sqsClient)

	// IgnoreEmptyResponses stops empty message outputs from reaching the core handler
	// and therefore the user can guarantee that there will be at least one message in
	// the message output.
	//
	// Note: Default poller comes with this middleware.
	poller.Use(sqspoller.IgnoreEmptyResponses())

	// supply handler to handle new messages
	poller.OnMessage(func(ctx context.Context, client *sqs.SQS, msgOutput *sqspoller.MessageOutput) error {
		// can guarantee messages will have length greater than or equal to one.
		msg := msgOutput.Messages[0]

		// delete message from queue
		if _, err := msg.Delete(); err != nil {
			return err
		}
		return nil
	})
}
```

## Shutdown

When shutting down the poller, there are three different modes of shutdown to choose from.

#### ShutdownNow

```go
 poller.ShutdownNow()
```

The ShutdownNow method cancels the context object immediately and exits the Run() function. It does not wait for any jobs to finish handling before exiting.

#### ShutdownGracefully

```go
 poller.ShutdownGracefully()
```

The ShutdownGracefully method waits for the handler to finish handling the current message before cancelling the context object and exiting the Run() function. If the handler is blocked then ShutdownGracefully will not exit.

#### ShutdownAfter

```go
 poller.ShutdownAfter(30*time.Second)
```

The ShutdownAfter method attempts to shutdown gracefully within the given time, if the handler cannot complete it's current job within the given time, then the context object is cancelled at that time allowing the Run() function to exit.

If the timeout happens before the poller can shutdown gracefully then ShutdownAfter returns error, ErrShutdownGraceful.

```go
func main() {
	poller := sqspoller.Default(sqsClient)

	poller.ReceiveMessageParams(&sqs.ReceiveMessageInput{
		MaxNumberOfMessages: aws.Int64(1),
		QueueUrl:            aws.String("https://sqs.us-east-2.amazonaws.com/123456789012/MyQueue"),
	})

	poller.OnMessage(messageHandler)
	poller.OnError(errorHandler)

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
```

## The Playground

To see how you can experiment and play around with a local SQS and poller instance, take a look [here](./cmd/playground).

## Go Version

-   Go Version 1.13+

## Testing

To run all the unit and integration tests, hit `make test`

Tests in the sqspoller_test.go file require that docker is installed and running on your machine,
as the tests spin up local SQS containers to test the framework against.
