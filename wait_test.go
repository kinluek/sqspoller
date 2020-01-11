package sqspoller

import (
	"testing"
	"time"
)

func Test_waitForInterval(t *testing.T) {

	t.Run("standard wait", func(t *testing.T) {
		if err := waitForInterval(time.Second, make(chan struct{})); err != nil {
			t.Fatalf("expected waitForInterval() to return nil, got %v", err)
		}
	})

	t.Run("exit before interval time", func(t *testing.T) {
		exit := make(chan struct{}, 1)
		exit <- struct{}{}
		if err := waitForInterval(time.Second, exit); err != nil {
			t.Fatalf("expected waitForInterval() to return nil, got %v", err)
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
