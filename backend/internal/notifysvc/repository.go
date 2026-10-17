// Package notifysvc is the ORION Notification service. It consumes domain
// events off the bus and turns them into customer notifications (RF009).
//
// In production the delivery step fans out through Amazon SNS (push, SMS,
// e-mail). Here delivery is simulated: the notification is persisted, logged,
// and pushed to connected clients by the gateway over WebSocket.
package notifysvc

import (
	"context"
	"sort"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/platform/httpx"
)

// Repository is the persistence contract of the Notification service.
type Repository interface {
	Create(ctx context.Context, notification domain.Notification) error
	ListByUser(ctx context.Context, userID string) ([]domain.Notification, error)
	MarkRead(ctx context.Context, id string) error
	MarkAllRead(ctx context.Context, userID string) error
	DeleteByUser(ctx context.Context, userID string) error
	Ping(ctx context.Context) error
}

// PostgresRepository stores notifications in the notify schema.
type PostgresRepository struct{ pool *pgxpool.Pool }

// NewPostgresRepository wires the repository to a pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// Create inserts a notification.
func (r *PostgresRepository) Create(ctx context.Context, notification domain.Notification) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO notify.notifications (id, user_id, title, body, channel, read, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		notification.ID, notification.UserID, notification.Title, notification.Body,
		notification.Channel, notification.Read, notification.CreatedAt)
	return err
}

// ListByUser returns the newest notifications of a customer.
func (r *PostgresRepository) ListByUser(ctx context.Context, userID string) ([]domain.Notification, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, title, body, channel, read, created_at
		  FROM notify.notifications
		 WHERE user_id = $1
		 ORDER BY created_at DESC
		 LIMIT 50`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notifications := make([]domain.Notification, 0)
	for rows.Next() {
		var notification domain.Notification
		if err := rows.Scan(&notification.ID, &notification.UserID, &notification.Title,
			&notification.Body, &notification.Channel, &notification.Read,
			&notification.CreatedAt); err != nil {
			return nil, err
		}
		notifications = append(notifications, notification)
	}
	return notifications, rows.Err()
}

// MarkRead flags one notification as read.
func (r *PostgresRepository) MarkRead(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE notify.notifications SET read = TRUE WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

// MarkAllRead clears the badge of a customer.
func (r *PostgresRepository) MarkAllRead(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE notify.notifications SET read = TRUE WHERE user_id = $1 AND read = FALSE`, userID)
	return err
}

// DeleteByUser erases a customer's notifications (LGPD).
func (r *PostgresRepository) DeleteByUser(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM notify.notifications WHERE user_id = $1`, userID)
	return err
}

// Ping backs the readiness probe.
func (r *PostgresRepository) Ping(ctx context.Context) error { return r.pool.Ping(ctx) }

// MemoryRepository is the dependency-free implementation.
type MemoryRepository struct {
	mu            sync.RWMutex
	notifications map[string]domain.Notification
}

// NewMemoryRepository builds an empty repository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{notifications: make(map[string]domain.Notification)}
}

// Create stores a notification.
func (r *MemoryRepository) Create(_ context.Context, notification domain.Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.notifications[notification.ID] = notification
	return nil
}

// ListByUser returns the newest notifications of a customer.
func (r *MemoryRepository) ListByUser(_ context.Context, userID string) ([]domain.Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]domain.Notification, 0)
	for _, notification := range r.notifications {
		if notification.UserID == userID {
			result = append(result, notification)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if len(result) > 50 {
		result = result[:50]
	}
	return result, nil
}

// MarkRead flags one notification as read.
func (r *MemoryRepository) MarkRead(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	notification, ok := r.notifications[id]
	if !ok {
		return httpx.ErrNotFound
	}
	notification.Read = true
	r.notifications[id] = notification
	return nil
}

// MarkAllRead clears the badge of a customer.
func (r *MemoryRepository) MarkAllRead(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, notification := range r.notifications {
		if notification.UserID == userID && !notification.Read {
			notification.Read = true
			r.notifications[id] = notification
		}
	}
	return nil
}

// DeleteByUser erases a customer's notifications (LGPD).
func (r *MemoryRepository) DeleteByUser(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, notification := range r.notifications {
		if notification.UserID == userID {
			delete(r.notifications, id)
		}
	}
	return nil
}

// Ping always succeeds: the store lives in this process.
func (r *MemoryRepository) Ping(context.Context) error { return nil }
