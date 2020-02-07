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
			time.Sleep(50 * time.Millisecond)
			errChan <- nil
		}()
		if err := waitForError(ctx, errChan); err != context.Canceled {
			t.Fatalf("expected waitForError() to return context.Canceled, got %v", err)
		}
	})
}

func Test_waitForInterval(t *testing.T) {

	t.Run("standard wait", func(t *testing.T) {
		if err := waitForInterval(context.Background(), time.Second, make(chan struct{})); err != nil {
			t.Fatalf("expected waitForInterval() to return nil, got %v", err)
		}
	})

	t.Run("exit before interval time", func(t *testing.T) {
		exit := make(chan struct{}, 1)
		exit <- struct{}{}
		if err := waitForInterval(context.Background(), time.Second, exit); err != nil {
			t.Fatalf("expected waitForInterval() to return nil, got %v", err)
		}
	})

	t.Run("context cancelled before interval time", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := waitForInterval(ctx, time.Second, make(chan struct{})); err != context.Canceled {
			t.Fatalf("expected waitForInterval() to return context.Canceled, got %v", err)
		}
	})

}

func Test_doubleWithLimit(t *testing.T) {
	tests := []struct {
		name    string
		current time.Duration
		limit   time.Duration
		want    time.Duration
	}{
		{
			name:    "current is 0",
			current: 0,
			limit:   10 * time.Second,
			want:    time.Second,
		},
		{
			name:    "current less than 1s",
			current: 20 * time.Millisecond,
			limit:   10 * time.Second,
			want:    time.Second,
		},
		{
			name:    "current is 1",
			current: time.Second,
			limit:   10 * time.Second,
			want:    2 * time.Second,
		},
		{
			name:    "current greater than 1",
			current: 3 * time.Second,
			limit:   10 * time.Second,
			want:    6 * time.Second,
		},
		{
			name:    "doubling current goes over limit",
			current: 6 * time.Second,
			limit:   10 * time.Second,
			want:    10 * time.Second,
		},
		{
			name:    "current is equal to limit",
			current: 10 * time.Second,
			limit:   10 * time.Second,
			want:    10 * time.Second,
		},
		{
			name:    "current greater than limit",
			current: 20 * time.Second,
			limit:   10 * time.Second,
			want:    10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if out := doubleWithLimit(tt.current, tt.limit); out != tt.want {
				t.Fatalf("doubleWithLimit expected %v, got %v", tt.want, out)
			}
		})
	}
}
