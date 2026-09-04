package gatewaysvc

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"

	"github.com/orion-cx/orion-backend/internal/authsvc"
	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/platform/httpx"
	"github.com/orion-cx/orion-backend/internal/platform/security"
)

// Routes is the public API. It is the only surface the channels talk to, which
// is the prototype equivalent of Amazon API Gateway in the production design.
func (s *Service) Routes() http.Handler {
	router := chi.NewRouter()

	router.Use(httpx.WithRequestID)
	router.Use(httpx.Recover(s.logger))
	router.Use(httpx.AccessLog(s.logger))
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.cfg.AllowedOrigin,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-Id"},
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, s.Health(r.Context()))
	})
	router.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		report, ready := s.Ready(r.Context())
		status := http.StatusOK
		if !ready {
			status = http.StatusServiceUnavailable
		}
		httpx.WriteJSON(w, status, report)
	})

	// The WebSocket handshake authenticates from the query string because the
	// browser API cannot set an Authorization header.
	router.Get("/ws", s.handleWebSocket)

	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", s.handleLogin)
		api.Post("/auth/register", s.handleRegister)

		api.Group(func(private chi.Router) {
			private.Use(s.tokens.Authenticated)

			private.Get("/auth/me", s.handleMe)
			private.Delete("/auth/me", s.handleForgetMe)

			private.Get("/session", s.handleSession)
			private.Get("/state", s.handleState)
			private.Get("/cases", s.handleState)
			private.Get("/cases/{id}", s.handleGetCase)
			private.Post("/cases/{id}/messages", s.handleCustomerMessage)
			private.Post("/cases/{id}/confirm-restart", s.handleConfirm)
			private.Post("/cases/{id}/decline-restart", s.handleDecline)
			private.Post("/cases/{id}/continue-here", s.handleConfirm)
			private.Post("/cases/{id}/switch-channel", s.handleSwitchChannel)
			private.Post("/cases/{id}/reset-conversation", s.handleResetConversation)

			private.Get("/tickets", s.handleTickets)
			private.Get("/recommendations", s.handleRecommendations)
			private.Get("/notifications", s.handleNotifications)
			private.Post("/notifications/{id}/read", s.handleNotificationRead)
			private.Post("/notifications/read-all", s.handleNotificationsReadAll)

			// Agent-only surface: the dashboard.
			private.Group(func(agent chi.Router) {
				agent.Use(security.RequireRole(domain.RoleAgent))

				agent.Post("/cases/{id}/take", s.handleTake)
				agent.Post("/cases/{id}/agent-messages", s.handleAgentMessage)
				agent.Post("/cases/{id}/resolve", s.handleResolve)
				agent.Post("/cases/{id}/mark-read", s.handleMarkRead)
				agent.Post("/dismiss-alert", s.handleDismissAlert)
			})
		})
	})

	return router
}

func principalOf(r *http.Request) (security.Principal, error) {
	principal, ok := security.FromContext(r.Context())
	if !ok {
		return security.Principal{}, httpx.ErrUnauthorized
	}
	return principal, nil
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, err)
		return
	}
	session, err := s.auth.Login(r.Context(), input.Email, input.Password)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, session)
}

func (s *Service) handleRegister(w http.ResponseWriter, r *http.Request) {
	var input authsvc.RegisterInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, err)
		return
	}
	// Self-service registration always creates a customer. Agent accounts are
	// provisioned by the seeder, never by the public API.
	input.Role = domain.RoleCustomer
	session, err := s.auth.Register(r.Context(), input)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, session)
}

func (s *Service) handleMe(w http.ResponseWriter, r *http.Request) {
	principal, err := principalOf(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	user, err := s.auth.Profile(r.Context(), principal.UserID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, user)
}

func (s *Service) handleForgetMe(w http.ResponseWriter, r *http.Request) {
	principal, err := principalOf(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if err := s.ForgetMe(r.Context(), principal); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"status":  "anonymized",
		"message": "Seus dados pessoais foram anonimizados e o histórico operacional foi removido.",
	})
}

// handleState returns the full snapshot the UI renders, and doubles as the
// bootstrap of a customer session: it opens a conversation on first contact.
func (s *Service) handleState(w http.ResponseWriter, r *http.Request) {
	principal, err := principalOf(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	channel := channelParam(r, domain.ChannelApp)
	if principal.Role != domain.RoleAgent {
		if _, err := s.ActiveConversation(r.Context(), principal, channel); err != nil {
			httpx.WriteError(w, err)
			return
		}
	}
	snapshot, err := s.Snapshot(r.Context(), principal.UserID, principal.Role)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, snapshot)
}

func (s *Service) handleSession(w http.ResponseWriter, r *http.Request) {
	principal, err := principalOf(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	session, found, err := s.sessions.Get(r.Context(), principal.UserID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if !found {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"found": false})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"found": true, "session": session})
}

func (s *Service) handleGetCase(w http.ResponseWriter, r *http.Request) {
	principal, err := principalOf(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	conversation, err := s.calls.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if conversation.UserID != principal.UserID && principal.Role != domain.RoleAgent {
		httpx.WriteError(w, httpx.ErrForbidden)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, conversation)
}

type messageInput struct {
	Text    string         `json:"text"`
	Channel domain.Channel `json:"channel"`
}

func (s *Service) handleCustomerMessage(w http.ResponseWriter, r *http.Request) {
	principal, err := principalOf(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	var input messageInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if input.Channel == "" {
		input.Channel = domain.ChannelApp
	}
	conversation, err := s.HandleCustomerMessage(r.Context(), principal, chi.URLParam(r, "id"), input.Text, input.Channel)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, conversation)
}

type channelInput struct {
	Channel         domain.Channel `json:"channel"`
	PreviousChannel domain.Channel `json:"previousChannel"`
}

func (s *Service) handleConfirm(w http.ResponseWriter, r *http.Request) {
	principal, err := principalOf(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	var input channelInput
	_ = httpx.DecodeJSON(r, &input)
	conversation, err := s.ConfirmPendingAction(r.Context(), principal, chi.URLParam(r, "id"), input.Channel)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, conversation)
}

func (s *Service) handleDecline(w http.ResponseWriter, r *http.Request) {
	principal, err := principalOf(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	var input channelInput
	_ = httpx.DecodeJSON(r, &input)
	conversation, err := s.DeclinePendingAction(r.Context(), principal, chi.URLParam(r, "id"), input.Channel)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, conversation)
}

func (s *Service) handleSwitchChannel(w http.ResponseWriter, r *http.Request) {
	principal, err := principalOf(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	var input channelInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, err)
		return
	}
	conversation, err := s.SwitchChannel(r.Context(), principal, chi.URLParam(r, "id"), input.PreviousChannel, input.Channel)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, conversation)
}

func (s *Service) handleResetConversation(w http.ResponseWriter, r *http.Request) {
	principal, err := principalOf(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	conversation, err := s.ResetConversation(r.Context(), principal, chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, conversation)
}

func (s *Service) handleTickets(w http.ResponseWriter, r *http.Request) {
	principal, err := principalOf(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	owner := principal.UserID
	if principal.Role == domain.RoleAgent {
		owner = r.URL.Query().Get("userId")
	}
	tickets, err := s.calls.Tickets(r.Context(), owner)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, tickets)
}

func (s *Service) handleRecommendations(w http.ResponseWriter, r *http.Request) {
	principal, err := principalOf(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	recommendations, err := s.Recommendations(r.Context(), principal.UserID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, recommendations)
}

func (s *Service) handleNotifications(w http.ResponseWriter, r *http.Request) {
	principal, err := principalOf(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	notifications, err := s.notifies.List(r.Context(), principal.UserID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, notifications)
}

func (s *Service) handleNotificationRead(w http.ResponseWriter, r *http.Request) {
	if err := s.notifies.MarkRead(r.Context(), chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "read"})
}

func (s *Service) handleNotificationsReadAll(w http.ResponseWriter, r *http.Request) {
	principal, err := principalOf(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	if err := s.notifies.MarkAllRead(r.Context(), principal.UserID); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "read"})
}

func (s *Service) handleTake(w http.ResponseWriter, r *http.Request) {
	principal, err := principalOf(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	conversation, err := s.AssignToAgent(r.Context(), principal, chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, conversation)
}

type agentMessageInput struct {
	Text string `json:"text"`
}

func (s *Service) handleAgentMessage(w http.ResponseWriter, r *http.Request) {
	principal, err := principalOf(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	var input agentMessageInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, err)
		return
	}
	conversation, err := s.AgentReply(r.Context(), principal, chi.URLParam(r, "id"), input.Text)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, conversation)
}

func (s *Service) handleResolve(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.ResolveConversation(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, conversation)
}

func (s *Service) handleMarkRead(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.MarkConversationRead(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, conversation)
}

func (s *Service) handleDismissAlert(w http.ResponseWriter, r *http.Request) {
	s.SetAlertVisible(r.Context(), false)
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"liveAlertVisible": false})
}

func (s *Service) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	token := security.BearerToken(r)
	if token == "" {
		httpx.WriteError(w, httpx.ErrUnauthorized)
		return
	}
	principal, err := s.tokens.Verify(token)
	if err != nil {
		httpx.WriteError(w, httpx.ErrUnauthorized)
		return
	}
	if err := s.hub.Serve(w, r, principal.UserID, string(principal.Role)); err != nil {
		s.logger.Warn("websocket upgrade failed", slog.String("err", err.Error()))
	}
}

// channelParam reads the channel a request came from, defaulting when absent.
func channelParam(r *http.Request, fallback domain.Channel) domain.Channel {
	channel := domain.Channel(r.URL.Query().Get("channel"))
	if channel.Valid() {
		return channel
	}
	return fallback
}
