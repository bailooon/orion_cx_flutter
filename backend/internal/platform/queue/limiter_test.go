package queue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestLimiterBoundsConcurrency(t *testing.T) {
	limiter := New("test", 2, 10, time.Second)

	var mu sync.Mutex
	current, peak := 0, 0
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := limiter.Acquire(context.Background())
			if err != nil {
				t.Errorf("unexpected rejection: %v", err)
				return
			}
			mu.Lock()
			current++
			if current > peak {
				peak = current
			}
			mu.Unlock()
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			current--
			mu.Unlock()
			release()
		}()
	}
	wg.Wait()

	if peak > 2 {
		t.Fatalf("expected at most 2 concurrent jobs, saw %d", peak)
	}
	if got := limiter.Stats().Processed; got != 8 {
		t.Fatalf("expected 8 processed jobs, got %d", got)
	}
}

// TestLimiterShedsWhenQueueIsFull covers the overload case: work beyond the
// queue is refused at once instead of piling up behind the busy agents.
func TestLimiterShedsWhenQueueIsFull(t *testing.T) {
	limiter := New("test", 1, 1, time.Second)

	holder, err := limiter.Acquire(context.Background())
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	// Second job waits in the single queue position.
	waiting := make(chan error, 1)
	go func() {
		release, err := limiter.Acquire(context.Background())
		if err == nil {
			release()
		}
		waiting <- err
	}()
	deadline := time.Now().Add(time.Second)
	for limiter.Stats().Queued < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	// Third job finds the queue full and is shed immediately.
	if _, err := limiter.Acquire(context.Background()); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("expected ErrQueueFull, got %v", err)
	}

	holder()
	if err := <-waiting; err != nil {
		t.Fatalf("queued job should have been admitted, got %v", err)
	}
	if stats := limiter.Stats(); stats.Rejected != 1 || stats.Processed != 2 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestLimiterTimesOutWaitingJobs(t *testing.T) {
	limiter := New("test", 1, 5, 20*time.Millisecond)
	holder, err := limiter.Acquire(context.Background())
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer holder()

	if _, err := limiter.Acquire(context.Background()); !errors.Is(err, ErrWaitTimeout) {
		t.Fatalf("expected ErrWaitTimeout, got %v", err)
	}
	if limiter.Stats().TimedOut != 1 {
		t.Fatal("expected the timeout to be counted")
	}
}

func TestReleaseIsIdempotent(t *testing.T) {
	limiter := New("test", 1, 0, time.Second)
	release, err := limiter.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	release()
	release()
	if _, err := limiter.Acquire(context.Background()); err != nil {
		t.Fatalf("slot should be free after release, got %v", err)
	}
	if limiter.Stats().InFlight != 1 {
		t.Fatal("double release must not free two slots")
	}
}
