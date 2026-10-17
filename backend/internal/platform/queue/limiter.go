// Package queue protects the AI agents from overload.
//
// A Limiter admits at most Capacity concurrent jobs and lets at most MaxQueue
// more wait for a slot, each for at most WaitTimeout. Anything beyond that is
// shed immediately instead of piling up: under a traffic spike the platform
// keeps answering with a degraded (deterministic) path rather than letting
// every request time out together. In production this sits in front of the
// model provider and is sized from its rate limits.
package queue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Errors returned when a job is not admitted.
var (
	// ErrQueueFull means the waiting line is already at MaxQueue.
	ErrQueueFull = errors.New("fila de agentes de IA cheia")
	// ErrWaitTimeout means the job waited WaitTimeout without getting a slot.
	ErrWaitTimeout = errors.New("tempo de espera na fila de agentes de IA esgotado")
)

// Stats is a point-in-time view of the limiter, shown on the dashboard.
type Stats struct {
	Name         string  `json:"name"`
	Capacity     int     `json:"capacity"`
	MaxQueue     int     `json:"maxQueue"`
	InFlight     int64   `json:"inFlight"`
	Queued       int64   `json:"queued"`
	Processed    int64   `json:"processed"`
	Rejected     int64   `json:"rejected"`
	TimedOut     int64   `json:"timedOut"`
	AvgWaitMs    float64 `json:"avgWaitMs"`
	AvgRunMs     float64 `json:"avgRunMs"`
	PeakInFlight int64   `json:"peakInFlight"`
	PeakQueued   int64   `json:"peakQueued"`
}

// Limiter is a bounded admission gate. It is safe for concurrent use.
type Limiter struct {
	name        string
	slots       chan struct{}
	maxQueue    int64
	waitTimeout time.Duration

	inFlight  atomic.Int64
	queued    atomic.Int64
	processed atomic.Int64
	rejected  atomic.Int64
	timedOut  atomic.Int64

	mu           sync.Mutex
	totalWait    time.Duration
	totalRun     time.Duration
	peakInFlight int64
	peakQueued   int64
}

// New builds a limiter. Non-positive values fall back to safe minimums.
func New(name string, capacity, maxQueue int, waitTimeout time.Duration) *Limiter {
	if capacity < 1 {
		capacity = 1
	}
	if maxQueue < 0 {
		maxQueue = 0
	}
	if waitTimeout <= 0 {
		waitTimeout = time.Second
	}
	return &Limiter{
		name:        name,
		slots:       make(chan struct{}, capacity),
		maxQueue:    int64(maxQueue),
		waitTimeout: waitTimeout,
	}
}

// Acquire waits for a slot. The returned release function must be called
// exactly once when the job finishes; it records the run time.
func (l *Limiter) Acquire(ctx context.Context) (func(), error) {
	started := time.Now()

	// Fast path: a free slot, no queueing.
	select {
	case l.slots <- struct{}{}:
		return l.admit(started), nil
	default:
	}

	// Slow path: join the line only if there is room in it.
	if l.queued.Add(1) > l.maxQueue {
		l.queued.Add(-1)
		l.rejected.Add(1)
		return nil, ErrQueueFull
	}
	l.notePeakQueued(l.queued.Load())

	timer := time.NewTimer(l.waitTimeout)
	defer timer.Stop()

	select {
	case l.slots <- struct{}{}:
		l.queued.Add(-1)
		return l.admit(started), nil
	case <-timer.C:
		l.queued.Add(-1)
		l.timedOut.Add(1)
		return nil, ErrWaitTimeout
	case <-ctx.Done():
		l.queued.Add(-1)
		l.timedOut.Add(1)
		return nil, ctx.Err()
	}
}

func (l *Limiter) admit(started time.Time) func() {
	waited := time.Since(started)
	current := l.inFlight.Add(1)
	l.mu.Lock()
	l.totalWait += waited
	if current > l.peakInFlight {
		l.peakInFlight = current
	}
	l.mu.Unlock()

	runStarted := time.Now()
	var once sync.Once
	return func() {
		once.Do(func() {
			l.inFlight.Add(-1)
			l.processed.Add(1)
			l.mu.Lock()
			l.totalRun += time.Since(runStarted)
			l.mu.Unlock()
			<-l.slots
		})
	}
}

func (l *Limiter) notePeakQueued(value int64) {
	l.mu.Lock()
	if value > l.peakQueued {
		l.peakQueued = value
	}
	l.mu.Unlock()
}

// Stats reports the current counters.
func (l *Limiter) Stats() Stats {
	l.mu.Lock()
	defer l.mu.Unlock()
	processed := l.processed.Load()
	stats := Stats{
		Name:         l.name,
		Capacity:     cap(l.slots),
		MaxQueue:     int(l.maxQueue),
		InFlight:     l.inFlight.Load(),
		Queued:       l.queued.Load(),
		Processed:    processed,
		Rejected:     l.rejected.Load(),
		TimedOut:     l.timedOut.Load(),
		PeakInFlight: l.peakInFlight,
		PeakQueued:   l.peakQueued,
	}
	admitted := processed + stats.InFlight
	if admitted > 0 {
		stats.AvgWaitMs = float64(l.totalWait.Microseconds()) / 1000 / float64(admitted)
	}
	if processed > 0 {
		stats.AvgRunMs = float64(l.totalRun.Microseconds()) / 1000 / float64(processed)
	}
	return stats
}
