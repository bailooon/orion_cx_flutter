// Package cache holds the low-latency session/context store that makes a
// journey survive a channel switch (RF003).
//
// Production uses Amazon DynamoDB. The prototype uses Redis, and falls back to
// an in-memory map when no Redis URL is configured. The key is the user id, not
// the channel: that single decision is what makes the context multichannel.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/orion-cx/orion-backend/internal/domain"
)

// TTL bounds how long an abandoned journey can be resumed.
const TTL = 24 * time.Hour

// SessionStore is the contract shared by the Redis and in-memory stores.
type SessionStore interface {
	Save(ctx context.Context, session domain.SessionContext) error
	Get(ctx context.Context, userID string) (domain.SessionContext, bool, error)
	Delete(ctx context.Context, userID string) error
	Ping(ctx context.Context) error
}

func key(userID string) string { return "orion:session:" + userID }

// Redis stores the session context in Redis with a TTL.
type Redis struct{ client *redis.Client }

// NewRedis parses a redis:// URL and builds a client.
func NewRedis(url string) (*Redis, error) {
	options, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	return &Redis{client: redis.NewClient(options)}, nil
}

// Save writes the session context, refreshing its TTL.
func (r *Redis) Save(ctx context.Context, session domain.SessionContext) error {
	session.UpdatedAt = time.Now().UTC()
	payload, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key(session.UserID), payload, TTL).Err()
}

// Get returns the stored context, if any.
func (r *Redis) Get(ctx context.Context, userID string) (domain.SessionContext, bool, error) {
	raw, err := r.client.Get(ctx, key(userID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return domain.SessionContext{}, false, nil
	}
	if err != nil {
		return domain.SessionContext{}, false, err
	}
	var session domain.SessionContext
	if err := json.Unmarshal(raw, &session); err != nil {
		return domain.SessionContext{}, false, err
	}
	return session, true, nil
}

// Delete drops the context, used by the LGPD erasure endpoint.
func (r *Redis) Delete(ctx context.Context, userID string) error {
	return r.client.Del(ctx, key(userID)).Err()
}

// Ping backs the readiness probe.
func (r *Redis) Ping(ctx context.Context) error { return r.client.Ping(ctx).Err() }

// Close releases the connection pool.
func (r *Redis) Close() error { return r.client.Close() }

type memoryEntry struct {
	session   domain.SessionContext
	expiresAt time.Time
}

// Memory is the dependency-free session store.
type Memory struct {
	mu      sync.RWMutex
	entries map[string]memoryEntry
}

// NewMemory builds the fallback store.
func NewMemory() *Memory {
	return &Memory{entries: make(map[string]memoryEntry)}
}

// Save writes the session context with the same TTL semantics as Redis.
func (m *Memory) Save(_ context.Context, session domain.SessionContext) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	session.UpdatedAt = time.Now().UTC()
	m.entries[session.UserID] = memoryEntry{session: session, expiresAt: time.Now().Add(TTL)}
	return nil
}

// Get returns the stored context, honouring expiry.
func (m *Memory) Get(_ context.Context, userID string) (domain.SessionContext, bool, error) {
	m.mu.RLock()
	entry, ok := m.entries[userID]
	m.mu.RUnlock()
	if !ok {
		return domain.SessionContext{}, false, nil
	}
	if time.Now().After(entry.expiresAt) {
		m.mu.Lock()
		delete(m.entries, userID)
		m.mu.Unlock()
		return domain.SessionContext{}, false, nil
	}
	return entry.session, true, nil
}

// Delete drops the context.
func (m *Memory) Delete(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, userID)
	return nil
}

// Ping always succeeds: the store lives in this process.
func (m *Memory) Ping(context.Context) error { return nil }
