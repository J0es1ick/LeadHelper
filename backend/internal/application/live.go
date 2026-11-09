package application

import (
	"context"
	"sync"
	"time"
)

// Wait for every worker at shutdown, including any in-flight source request.
func runPollJobs(ctx context.Context, jobs ...func(context.Context)) {
	var wg sync.WaitGroup
	for _, job := range jobs {
		wg.Add(1)
		go func() { defer wg.Done(); job(ctx) }()
	}
	wg.Wait()
}

// Schedule after completion rather than accumulating ticks while a service is
// slow. Failed providers back off independently; manual refresh stays available.
func pollSource(ctx context.Context, interval, timeout time.Duration, fetch func(context.Context) error) {
	delay := interval
	for ctx.Err() == nil {
		request, cancel := context.WithTimeout(ctx, timeout)
		err := fetch(request)
		cancel()
		if err != nil {
			delay = min(delay*2, 5*time.Minute)
		} else {
			delay = interval
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
