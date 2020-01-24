package sqspoller

import (
	"context"
	"errors"
	"testing"
	"time"
)

func Test_waitForError(t *testing.T) {

	t.Run("nil error from channel", func(t *testing.T) {
		errChan := make(chan error)
		go func() {
			errChan <- nil
		}()
		if err := waitForError(context.Background(), errChan); err != nil {
			t.Fatalf("expected waitForError() to return nil, got %v", err)
		}
	})

	t.Run("non-nil error from channel", func(t *testing.T) {
		errChan := make(chan error)
		ErrConfirmed := errors.New("confirmed")
		go func() {
			errChan <- ErrConfirmed
		}()
		if err := waitForError(context.Background(), errChan); err != ErrConfirmed {
			t.Fatalf("expected waitForError() to return ErrConfirmed, got %v", err)
		}
	})

	t.Run("context cancelled error", func(t *testing.T) {
		errChan := make(chan error)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			cancel()
			errChan <- nil
		}()
		if err := waitForError(ctx, errChan); err != context.Canceled {
			t.Fatalf("expected waitForError() to return context.Canceled, got %v", err)
		}
	})

	t.Run("error from error channel after context cancelled", func(t *testing.T) {
		errChan := make(chan error)
		ctx, cancel := context.WithCancel(context.Background())
		ErrConfirmed := errors.New("confirmed")
		go func() {
			cancel()
			time.Sleep(50 * time.Millisecond)
			errChan <- ErrConfirmed
		}()
		if err := waitForError(ctx, errChan); err != ErrConfirmed {
			t.Fatalf("expected waitForError() to return ErrConfirmed, got %v", err)
		}
	})

}
