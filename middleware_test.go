package sqspoller

import (
	"context"
	"errors"
	"testing"
)

func Test_wrapMiddleware(t *testing.T) {

	// ==============================================================
	// Assert that logic happens in the expected order, when using
	// wrapMiddleware function.

	want := "12handler21"
	text := ""

	middleWare1 := func(handler Handler) Handler {
		h := func(ctx context.Context, msg *MessageOutput, err error) error {
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
		h := func(ctx context.Context, msg *MessageOutput, err error) error {
			text += "2"
			if err := handler(ctx, msg, err); err != nil {
				return err
			}
			text += "2"
			return nil
		}
		return h
	}

	handler := func(ctx context.Context, msg *MessageOutput, err error) error {
		text += "handler"
		return nil
	}

	wrappedHandler := wrapMiddleware([]Middleware{middleWare1, middleWare2}, handler)

	if err := wrappedHandler(context.Background(), &MessageOutput{}, nil); err != nil {
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
		h := func(ctx context.Context, msg *MessageOutput, err error) error {
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
		h := func(ctx context.Context, msg *MessageOutput, err error) error {
			text += "2"
			if err := handler(ctx, msg, err); err != nil {
				return err
			}
			text += "2"
			return nil
		}
		return h
	}

	handler := func(ctx context.Context, msg *MessageOutput, err error) error {
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
	if err := wrappedHandler(context.Background(), &MessageOutput{}, nil); err != nil {
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
	if err := wrappedHandler(context.Background(), &MessageOutput{}, nil); err != nil {
		t.Fatalf("wrappedHandler should not have returned an error: %v", err)
	}

	if text != want {
		t.Fatalf("final text when using Use consecutively: %v wanted: %v", text, want)
	}

}

func TestIgnoreEmptyResponses(t *testing.T) {

	tests := []struct {
		name        string
		wantReached bool
		messages    []*Message
		Err         error
	}{
		{
			name:        "nil messages",
			wantReached: false,
			messages:    nil,
			Err:         nil,
		},
		{
			name:        "no messages",
			wantReached: false,
			messages:    make([]*Message, 0),
			Err:         nil,
		},
		{
			name:        "has messages",
			wantReached: true,
			messages:    make([]*Message, 1),
			Err:         nil,
		},
		{
			name:        "no messages with error",
			wantReached: true,
			messages:    make([]*Message, 0),
			Err:         errors.New("error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var innerReached bool

			inner := func(ctx context.Context, msgOutput *MessageOutput, err error) error {
				innerReached = true
				return errors.New("inner error")
			}

			handler := IgnoreEmptyResponses()(inner)
			handler(context.Background(), &MessageOutput{Messages: tt.messages}, tt.Err)

			if innerReached != tt.wantReached {
				t.Fatalf("wanted wantReached to be %v, got %v", tt.wantReached, innerReached)
			}
		})
	}

}

