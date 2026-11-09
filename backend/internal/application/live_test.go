package application

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestLiveWorkersRunIndependentlyAndStopTogether(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	slowStarted := make(chan struct{})
	fastRepeated := make(chan struct{})
	stopped := make(chan struct{})
	var calls atomic.Int32
	go func() {
		runPollJobs(ctx,
			func(ctx context.Context) {
				pollSource(ctx, time.Millisecond, time.Second, func(c context.Context) error {
					close(slowStarted)
					<-c.Done()
					return c.Err()
				})
			},
			func(ctx context.Context) {
				pollSource(ctx, time.Millisecond, time.Second, func(c context.Context) error {
					if calls.Add(1) == 3 {
						close(fastRepeated)
					}
					return nil
				})
			},
		)
		close(stopped)
	}()
	select {
	case <-slowStarted:
	case <-time.After(time.Second):
		t.Fatal("slow provider did not start")
	}
	select {
	case <-fastRepeated:
	case <-time.After(time.Second):
		t.Fatal("slow provider blocked other updates")
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("workers did not stop with server")
	}
}

func TestLiveRequestDeadlineAndNoOverlap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	var calls, active atomic.Int32
	go func() {
		pollSource(ctx, time.Millisecond, 5*time.Millisecond, func(c context.Context) error {
			if active.Add(1) != 1 {
				t.Error("overlapping requests")
			}
			defer active.Add(-1)
			<-c.Done()
			if c.Err() != context.DeadlineExceeded {
				t.Error("missing request timeout")
			}
			if calls.Add(1) == 2 {
				cancel()
			}
			return c.Err()
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stalled request stopped automatic updates")
	}
	if calls.Load() != 2 {
		t.Fatal("expected recovery after deadline")
	}
}
