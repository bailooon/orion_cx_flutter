package security

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/platform/httpx"
)

func testUser() domain.User {
	return domain.User{ID: "USR-1", Email: "cliente@orion.dev", Name: "Cliente Demo", Role: domain.RoleCustomer}
}

func TestPasswordIsHashedNotStored(t *testing.T) {
	const plain = "orion12345"
	hash, err := HashPassword(plain, 4)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == plain {
		t.Fatal("the password must never be stored in clear text")
	}
	if !CheckPassword(hash, plain) {
		t.Fatal("the correct password must verify")
	}
	if CheckPassword(hash, "senha-errada") {
		t.Fatal("a wrong password must not verify")
	}

	// bcrypt salts every digest, so the same password hashes differently.
	other, err := HashPassword(plain, 4)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if other == hash {
		t.Fatal("expected a per-hash salt")
	}
}

func TestTokenRoundTrip(t *testing.T) {
	tokens := NewTokens("secret-a", time.Hour)
	token, expiresAt, err := tokens.Issue(testUser())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if !expiresAt.After(time.Now()) {
		t.Fatal("the token must expire in the future")
	}

	principal, err := tokens.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if principal.UserID != "USR-1" || principal.Role != domain.RoleCustomer {
		t.Fatalf("unexpected principal: %+v", principal)
	}
}

func TestTokenSignedWithAnotherSecretIsRejected(t *testing.T) {
	token, _, err := NewTokens("secret-a", time.Hour).Issue(testUser())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := NewTokens("secret-b", time.Hour).Verify(token); err == nil {
		t.Fatal("a token signed with another secret must be rejected")
	}
}

func TestExpiredTokenIsRejected(t *testing.T) {
	tokens := NewTokens("secret-a", -time.Minute)
	token, _, err := tokens.Issue(testUser())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := tokens.Verify(token); err == nil {
		t.Fatal("an expired token must be rejected")
	}
}

func TestTamperedTokenIsRejected(t *testing.T) {
	tokens := NewTokens("secret-a", time.Hour)
	token, _, err := tokens.Issue(testUser())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := tokens.Verify(token + "x"); err == nil {
		t.Fatal("a tampered token must be rejected")
	}
	if _, err := tokens.Verify(""); err == nil {
		t.Fatal("an empty token must be rejected")
	}
}

func TestAuthenticatedMiddleware(t *testing.T) {
	tokens := NewTokens("secret-a", time.Hour)
	token, _, err := tokens.Issue(testUser())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	var seen Principal
	handler := tokens.Authenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("rejects a request without a token", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/state", nil))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", recorder.Code)
		}
	})

	t.Run("accepts a bearer token", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/state", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", recorder.Code)
		}
		if seen.UserID != "USR-1" {
			t.Fatalf("expected the principal on the context, got %+v", seen)
		}
	})

	t.Run("accepts the websocket query token", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/ws?token="+token, nil)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", recorder.Code)
		}
	})
}

func TestRequireRole(t *testing.T) {
	handler := RequireRole(domain.RoleAgent)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	customer := httptest.NewRequest(http.MethodPost, "/api/cases/1/take", nil)
	customer = customer.WithContext(WithPrincipal(customer.Context(), Principal{Role: domain.RoleCustomer}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, customer)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a customer, got %d", recorder.Code)
	}

	agent := httptest.NewRequest(http.MethodPost, "/api/cases/1/take", nil)
	agent = agent.WithContext(WithPrincipal(agent.Context(), Principal{Role: domain.RoleAgent}))
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, agent)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for an agent, got %d", recorder.Code)
	}
}

func TestVerifyReturnsAPIUnauthorized(t *testing.T) {
	_, err := NewTokens("secret", time.Hour).Verify("not-a-token")
	var apiErr httpx.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized {
		t.Fatalf("expected an unauthorized API error, got %v", err)
	}
}
