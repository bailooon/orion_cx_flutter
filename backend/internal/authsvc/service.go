package authsvc

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/platform/httpx"
	"github.com/orion-cx/orion-backend/internal/platform/security"
)

// Service implements the authentication use cases.
type Service struct {
	repo       Repository
	tokens     *security.Tokens
	bcryptCost int
	logger     *slog.Logger
}

// NewService wires the Authenticator.
func NewService(repo Repository, tokens *security.Tokens, bcryptCost int, logger *slog.Logger) *Service {
	return &Service{repo: repo, tokens: tokens, bcryptCost: bcryptCost, logger: logger}
}

// RegisterInput is the payload of a sign-up.
type RegisterInput struct {
	Email        string      `json:"email"`
	Password     string      `json:"password"`
	Name         string      `json:"name"`
	DocumentMask string      `json:"documentMask"`
	PlanName     string      `json:"planName"`
	Role         domain.Role `json:"role"`
}

// Session is what a successful login returns: the token plus the profile the
// UI needs, never the password digest.
type Session struct {
	Token     string      `json:"token"`
	ExpiresAt time.Time   `json:"expiresAt"`
	User      domain.User `json:"user"`
}

// Register creates a user and immediately returns a usable session.
func (s *Service) Register(ctx context.Context, input RegisterInput) (Session, error) {
	email := normalizeEmail(input.Email)
	if !strings.Contains(email, "@") {
		return Session{}, httpx.BadRequest("Informe um e-mail válido.")
	}
	if len(input.Password) < 8 {
		return Session{}, httpx.BadRequest("A senha deve ter ao menos 8 caracteres.")
	}
	if strings.TrimSpace(input.Name) == "" {
		return Session{}, httpx.BadRequest("Informe o nome do titular.")
	}
	role := input.Role
	if role != domain.RoleAgent {
		role = domain.RoleCustomer
	}

	hash, err := security.HashPassword(input.Password, s.bcryptCost)
	if err != nil {
		return Session{}, err
	}

	user := domain.User{
		ID:           "USR-" + uuid.NewString()[:8],
		Email:        email,
		Name:         strings.TrimSpace(input.Name),
		DocumentMask: input.DocumentMask,
		PlanName:     input.PlanName,
		Role:         role,
		PasswordHash: hash,
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.repo.Create(ctx, user); err != nil {
		return Session{}, err
	}
	s.logger.Info("user registered", slog.String("user_id", user.ID), slog.String("role", string(user.Role)))
	return s.newSession(user)
}

// Login validates credentials and issues a token valid on every channel.
func (s *Service) Login(ctx context.Context, email, password string) (Session, error) {
	user, err := s.repo.ByEmail(ctx, email)
	if err != nil {
		// Same error for unknown user and wrong password: revealing which one
		// failed would let an attacker enumerate accounts.
		return Session{}, httpx.ErrUnauthorized
	}
	if user.Anonymized() || !security.CheckPassword(user.PasswordHash, password) {
		return Session{}, httpx.ErrUnauthorized
	}
	s.logger.Info("login succeeded", slog.String("user_id", user.ID))
	return s.newSession(user)
}

func (s *Service) newSession(user domain.User) (Session, error) {
	token, expiresAt, err := s.tokens.Issue(user)
	if err != nil {
		return Session{}, err
	}
	user.PasswordHash = ""
	return Session{Token: token, ExpiresAt: expiresAt, User: user}, nil
}

// Profile returns the user behind an id, with the digest stripped.
func (s *Service) Profile(ctx context.Context, id string) (domain.User, error) {
	user, err := s.repo.ByID(ctx, id)
	if err != nil {
		return domain.User{}, err
	}
	user.PasswordHash = ""
	return user, nil
}

// Anonymize fulfils an LGPD erasure request for the given user.
func (s *Service) Anonymize(ctx context.Context, id string) error {
	if err := s.repo.Anonymize(ctx, id); err != nil {
		return err
	}
	s.logger.Info("user anonymized", slog.String("user_id", id))
	return nil
}

// EnsureUser creates a user if the email is free and returns it either way.
// The seeder uses it so restarting the stack never duplicates demo accounts.
func (s *Service) EnsureUser(ctx context.Context, id string, input RegisterInput) (domain.User, error) {
	if existing, err := s.repo.ByEmail(ctx, input.Email); err == nil {
		return existing, nil
	}
	hash, err := security.HashPassword(input.Password, s.bcryptCost)
	if err != nil {
		return domain.User{}, err
	}
	user := domain.User{
		ID:           id,
		Email:        normalizeEmail(input.Email),
		Name:         input.Name,
		DocumentMask: input.DocumentMask,
		PlanName:     input.PlanName,
		Role:         input.Role,
		PasswordHash: hash,
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.repo.Create(ctx, user); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

// Count reports how many users exist, used to decide whether to seed.
func (s *Service) Count(ctx context.Context) (int, error) { return s.repo.Count(ctx) }
