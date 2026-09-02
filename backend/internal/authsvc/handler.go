package authsvc

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/orion-cx/orion-backend/internal/platform/httpx"
)

// Routes mounts the internal API of the Authenticator service. These endpoints
// are reached by the gateway, never directly by a channel.
func (s *Service) Routes() http.Handler {
	router := chi.NewRouter()

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		if _, err := s.repo.Count(r.Context()); err != nil {
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded"})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "orion-authenticator"})
	})

	router.Post("/v1/register", s.handleRegister)
	router.Post("/v1/login", s.handleLogin)
	router.Get("/v1/users/{id}", s.handleProfile)
	router.Delete("/v1/users/{id}", s.handleAnonymize)

	return router
}

func (s *Service) handleRegister(w http.ResponseWriter, r *http.Request) {
	var input RegisterInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, err)
		return
	}
	session, err := s.Register(r.Context(), input)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, session)
}

type loginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	var input loginInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, err)
		return
	}
	session, err := s.Login(r.Context(), input.Email, input.Password)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, session)
}

func (s *Service) handleProfile(w http.ResponseWriter, r *http.Request) {
	user, err := s.Profile(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, user)
}

func (s *Service) handleAnonymize(w http.ResponseWriter, r *http.Request) {
	if err := s.Anonymize(r.Context(), chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "anonymized"})
}
