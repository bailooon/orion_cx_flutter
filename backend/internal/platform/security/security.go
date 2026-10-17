// Package security implements the cross-cutting parts of RNF004: password
// hashing, signed access tokens and the middleware that guards protected
// routes. The same token is accepted by every channel, which is what makes
// authentication multichannel (RF001).
package security

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/platform/httpx"
)

type ctxKey string

const principalKey ctxKey = "orion.principal"

// Principal is the authenticated identity extracted from a valid token.
type Principal struct {
	UserID string      `json:"userId"`
	Email  string      `json:"email"`
	Name   string      `json:"name"`
	Role   domain.Role `json:"role"`
}

// Claims is the JWT payload Orion issues.
type Claims struct {
	jwt.RegisteredClaims
	Email string      `json:"email"`
	Name  string      `json:"name"`
	Role  domain.Role `json:"role"`
}

// Tokens issues and verifies access tokens with a shared secret. In production
// this would be an asymmetric key managed by AWS KMS; the verification path is
// identical, only the key material changes.
type Tokens struct {
	secret []byte
	ttl    time.Duration
}

// NewTokens builds a token issuer.
func NewTokens(secret string, ttl time.Duration) *Tokens {
	return &Tokens{secret: []byte(secret), ttl: ttl}
}

// TTL exposes the configured token lifetime so the API can tell the client
// when to re-authenticate.
func (t *Tokens) TTL() time.Duration { return t.ttl }

// Issue signs a token for the given user.
func (t *Tokens) Issue(user domain.User) (string, time.Time, error) {
	expiresAt := time.Now().Add(t.ttl)
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			Issuer:    "orion-cx",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
		Email: user.Email,
		Name:  user.Name,
		Role:  user.Role,
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

// Verify validates the signature and expiry of a token and returns its
// principal. Expired or tampered tokens produce ErrUnauthorized.
func (t *Tokens) Verify(token string) (Principal, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(candidate *jwt.Token) (any, error) {
		if _, ok := candidate.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, httpx.ErrUnauthorized
		}
		return t.secret, nil
	}, jwt.WithIssuer("orion-cx"), jwt.WithExpirationRequired())
	if err != nil {
		return Principal{}, httpx.ErrUnauthorized
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return Principal{}, httpx.ErrUnauthorized
	}
	return Principal{
		UserID: claims.Subject,
		Email:  claims.Email,
		Name:   claims.Name,
		Role:   claims.Role,
	}, nil
}

// HashPassword stores a password as a bcrypt digest. Plaintext passwords are
// never persisted or logged.
func HashPassword(plain string, cost int) (string, error) {
	digest, err := bcrypt.GenerateFromPassword([]byte(plain), cost)
	if err != nil {
		return "", err
	}
	return string(digest), nil
}

// CheckPassword compares a candidate password against a stored digest.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// FromContext returns the authenticated principal placed by Authenticated.
func FromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalKey).(Principal)
	return principal, ok
}

// WithPrincipal is used by the middleware and by tests to inject an identity.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalKey, principal)
}

// BearerToken extracts the raw token from the Authorization header.
func BearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if header == "" {
		// The browser WebSocket API cannot set headers, so the dashboard
		// passes its token as a query parameter on the /ws handshake only.
		return strings.TrimSpace(r.URL.Query().Get("token"))
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// Authenticated rejects requests without a valid token.
func (t *Tokens) Authenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := BearerToken(r)
		if token == "" {
			httpx.WriteError(w, httpx.ErrUnauthorized)
			return
		}
		principal, err := t.Verify(token)
		if err != nil {
			httpx.WriteError(w, httpx.ErrUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
	})
}

// RequireRole guards routes that only one profile may reach, such as the agent
// dashboard.
func RequireRole(role domain.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := FromContext(r.Context())
			if !ok {
				httpx.WriteError(w, httpx.ErrUnauthorized)
				return
			}
			if principal.Role != role {
				httpx.WriteError(w, httpx.ErrForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
