package sqspoller

import (
	"context"
	"github.com/aws/aws-sdk-go/service/sqs"
	"testing"
)

func Test_wrapMiddleware(t *testing.T) {

	// ==============================================================
	// Assert that logic happens in the expected order, when using
	// wrapMiddleware function.

	want := "12handler21"
	text := ""

	middleWare1 := func(handler Handler) Handler {
		h := func(ctx context.Context, msg *sqs.ReceiveMessageOutput, err error) error {
			text += "1"
			if err := handler(ctx, msg, err); err != nil {
				return err
			}
			text += "1"
			return nil
		}
		return h
	}

	middleWare2 := func(handler Handler) Handler {
		h := func(ctx context.Context, msg *sqs.ReceiveMessageOutput, err error) error {
			text += "2"
			if err := handler(ctx, msg, err); err != nil {
				return err
			}
			text += "2"
			return nil
		}
		return h
	}

	handler := func(ctx context.Context, msg *sqs.ReceiveMessageOutput, err error) error {
		text += "handler"
		return nil
	}

	wrappedHandler := wrapMiddleware([]Middleware{middleWare1, middleWare2}, handler)

	if err := wrappedHandler(context.Background(), &sqs.ReceiveMessageOutput{}, nil); err != nil {
		t.Fatalf("wrappedHandler should not have returned an error: %v", err)
	}

	if text != want {
		t.Fatalf("final text produced by wrapped handler: %v wanted: %v", text, want)
	}

}

func TestPoller_Use(t *testing.T) {
	want := "12handler21"

	text := ""

	middleWare1 := func(handler Handler) Handler {
		h := func(ctx context.Context, msg *sqs.ReceiveMessageOutput, err error) error {
			text += "1"
			if err := handler(ctx, msg, err); err != nil {
				return err
			}
			text += "1"
			return nil
		}
		return h
	}

	middleWare2 := func(handler Handler) Handler {
		h := func(ctx context.Context, msg *sqs.ReceiveMessageOutput, err error) error {
			text += "2"
			if err := handler(ctx, msg, err); err != nil {
				return err
			}
			text += "2"
			return nil
		}
		return h
	}

	handler := func(ctx context.Context, msg *sqs.ReceiveMessageOutput, err error) error {
		text += "handler"
		return nil
	}

	// ==============================================================
	// Assert that logic happens in the expected order, when using
	// Use method to use multiple middleware at once.

	poller := &Poller{}
	poller.Use(middleWare1, middleWare2)
	poller.Handle(handler)

	wrappedHandler := poller.handler
	if err := wrappedHandler(context.Background(), &sqs.ReceiveMessageOutput{}, nil); err != nil {
		t.Fatalf("wrappedHandler should not have returned an error: %v", err)
	}

	if text != want {
		t.Fatalf("final text using Use to wrap multiple middleware at once: %v wanted: %v", text, want)
	}

	// ==============================================================
	// Assert that logic happens in the expected order, when using
	// Use consecutively.

	text = ""

	poller = &Poller{}
	poller.Use(middleWare1)
	poller.Use(middleWare2)
	poller.Handle(handler)

	wrappedHandler = poller.handler
	if err := wrappedHandler(context.Background(), &sqs.ReceiveMessageOutput{}, nil); err != nil {
		t.Fatalf("wrappedHandler should not have returned an error: %v", err)
	}

	if text != want {
		t.Fatalf("final text when using Use consecutively: %v wanted: %v", text, want)
	}

}
