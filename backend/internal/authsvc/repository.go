// Package authsvc is the ORION Authenticator: it owns the user records and
// issues the access token that every channel presents (RF001).
package authsvc

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/platform/httpx"
)

// ErrEmailTaken is returned when a registration collides with an existing user.
var ErrEmailTaken = httpx.Conflict("Já existe uma conta com este e-mail.")

// Repository is the persistence contract of the Authenticator service.
type Repository interface {
	Create(ctx context.Context, user domain.User) error
	ByEmail(ctx context.Context, email string) (domain.User, error)
	ByID(ctx context.Context, id string) (domain.User, error)
	List(ctx context.Context) ([]domain.User, error)
	// Anonymize wipes the identifying columns but keeps the row, so protocols
	// that reference the user stay auditable (LGPD).
	Anonymize(ctx context.Context, id string) error
	Count(ctx context.Context) (int, error)
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// PostgresRepository stores users in the auth schema.
type PostgresRepository struct{ pool *pgxpool.Pool }

// NewPostgresRepository wires the repository to a pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const userColumns = `id, email, name, document_mask, plan_name, role, password_hash, created_at, anonymized_at`

func scanUser(row pgx.Row) (domain.User, error) {
	var user domain.User
	err := row.Scan(
		&user.ID, &user.Email, &user.Name, &user.DocumentMask,
		&user.PlanName, &user.Role, &user.PasswordHash, &user.CreatedAt, &user.AnonymizedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, httpx.ErrNotFound
	}
	return user, err
}

// Create inserts a user, mapping a unique violation to ErrEmailTaken.
func (r *PostgresRepository) Create(ctx context.Context, user domain.User) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO auth.users (`+userColumns+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		user.ID, normalizeEmail(user.Email), user.Name, user.DocumentMask,
		user.PlanName, user.Role, user.PasswordHash, user.CreatedAt, user.AnonymizedAt,
	)
	if err != nil && strings.Contains(err.Error(), "users_email_key") {
		return ErrEmailTaken
	}
	return err
}

// ByEmail looks a user up by their login identifier.
func (r *PostgresRepository) ByEmail(ctx context.Context, email string) (domain.User, error) {
	return scanUser(r.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM auth.users WHERE email = $1`, normalizeEmail(email)))
}

// ByID looks a user up by primary key.
func (r *PostgresRepository) ByID(ctx context.Context, id string) (domain.User, error) {
	return scanUser(r.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM auth.users WHERE id = $1`, id))
}

// List returns every user, used by the seeder and the admin roster.
func (r *PostgresRepository) List(ctx context.Context) ([]domain.User, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+userColumns+` FROM auth.users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []domain.User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

// Anonymize implements the LGPD erasure request.
func (r *PostgresRepository) Anonymize(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE auth.users
		   SET email         = 'anonimizado+' || id || '@orion.local',
		       name          = 'Titular anonimizado',
		       document_mask = '',
		       plan_name     = '',
		       password_hash = '',
		       anonymized_at = now()
		 WHERE id = $1 AND anonymized_at IS NULL`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

// Count backs the seeder decision of whether to populate demo data.
func (r *PostgresRepository) Count(ctx context.Context) (int, error) {
	var total int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM auth.users`).Scan(&total)
	return total, err
}

// MemoryRepository is the dependency-free implementation used when no Postgres
// URL is configured. It is also what the unit tests run against.
type MemoryRepository struct {
	mu    sync.RWMutex
	users map[string]domain.User
}

// NewMemoryRepository builds an empty in-memory repository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{users: make(map[string]domain.User)}
}

// Create stores a user unless the email is taken.
func (r *MemoryRepository) Create(_ context.Context, user domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	user.Email = normalizeEmail(user.Email)
	for _, existing := range r.users {
		if existing.Email == user.Email {
			return ErrEmailTaken
		}
	}
	r.users[user.ID] = user
	return nil
}

// ByEmail looks a user up by their login identifier.
func (r *MemoryRepository) ByEmail(_ context.Context, email string) (domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	target := normalizeEmail(email)
	for _, user := range r.users {
		if user.Email == target {
			return user, nil
		}
	}
	return domain.User{}, httpx.ErrNotFound
}

// ByID looks a user up by primary key.
func (r *MemoryRepository) ByID(_ context.Context, id string) (domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if user, ok := r.users[id]; ok {
		return user, nil
	}
	return domain.User{}, httpx.ErrNotFound
}

// List returns every user ordered by creation time.
func (r *MemoryRepository) List(_ context.Context) ([]domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	users := make([]domain.User, 0, len(r.users))
	for _, user := range r.users {
		users = append(users, user)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].CreatedAt.Before(users[j].CreatedAt) })
	return users, nil
}

// Anonymize implements the LGPD erasure request.
func (r *MemoryRepository) Anonymize(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	user, ok := r.users[id]
	if !ok || user.Anonymized() {
		return httpx.ErrNotFound
	}
	now := time.Now().UTC()
	user.Email = "anonimizado+" + user.ID + "@orion.local"
	user.Name = "Titular anonimizado"
	user.DocumentMask = ""
	user.PlanName = ""
	user.PasswordHash = ""
	user.AnonymizedAt = &now
	r.users[id] = user
	return nil
}

// Count reports how many users exist.
func (r *MemoryRepository) Count(_ context.Context) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.users), nil
}
