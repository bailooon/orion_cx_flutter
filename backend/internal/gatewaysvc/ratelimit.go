package gatewaysvc

import (
	"sync"
	"time"
)

// rateLimiter is a per-customer token bucket. It stops a single account (or a
// script holding its token) from flooding the agents, independently of the
// global agent queue, which protects the platform as a whole.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	rate    float64 // tokens per second
	burst   float64
}

type tokenBucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(perMinute, burst int) *rateLimiter {
	if perMinute < 1 {
		perMinute = 1
	}
	if burst < 1 {
		burst = 1
	}
	return &rateLimiter{
		buckets: make(map[string]*tokenBucket),
		rate:    float64(perMinute) / 60,
		burst:   float64(burst),
	}
}

// Allow consumes one token for key, reporting whether the call may proceed.
func (r *rateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	bucket, ok := r.buckets[key]
	if !ok {
		bucket = &tokenBucket{tokens: r.burst, last: now}
		r.buckets[key] = bucket
		// Opportunistic cleanup keeps the map bounded without a goroutine.
		if len(r.buckets) > 10000 {
			for k, b := range r.buckets {
				if now.Sub(b.last) > 10*time.Minute {
					delete(r.buckets, k)
				}
			}
		}
	}
	bucket.tokens += now.Sub(bucket.last).Seconds() * r.rate
	if bucket.tokens > r.burst {
		bucket.tokens = r.burst
	}
	bucket.last = now
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}
