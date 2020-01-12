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

