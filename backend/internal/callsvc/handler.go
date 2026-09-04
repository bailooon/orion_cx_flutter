package callsvc

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/platform/httpx"
)

// Routes mounts the internal API of Call Management, consumed by the gateway.
func (s *Service) Routes() http.Handler {
	router := chi.NewRouter()

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := s.repo.Ping(r.Context()); err != nil {
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded"})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "orion-call-management"})
	})

	router.Route("/v1/conversations", func(sub chi.Router) {
		sub.Post("/", s.handleOpenConversation)
		sub.Get("/", s.handleListConversations)
		sub.Get("/{id}", s.handleGetConversation)
		sub.Post("/{id}/turns", s.handleApplyTurn)
		sub.Post("/{id}/assign", s.handleAssign)
		sub.Post("/{id}/agent-messages", s.handleAgentReply)
		sub.Post("/{id}/resolve", s.handleResolve)
		sub.Post("/{id}/mark-read", s.handleMarkRead)
		sub.Post("/{id}/reset", s.handleReset)
	})

	router.Route("/v1/tickets", func(sub chi.Router) {
		sub.Post("/", s.handleOpenTicket)
		sub.Get("/", s.handleListTickets)
		sub.Get("/{id}", s.handleGetTicket)
		sub.Patch("/{id}", s.handleUpdateTicket)
	})

	router.Delete("/v1/users/{userId}/data", s.handlePurgeUser)

	return router
}

func (s *Service) handleOpenConversation(w http.ResponseWriter, r *http.Request) {
	var input NewConversationInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, err)
		return
	}
	conversation, err := s.OpenConversation(r.Context(), input)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, conversation)
}

func (s *Service) handleListConversations(w http.ResponseWriter, r *http.Request) {
	filter := ConversationFilter{UserID: r.URL.Query().Get("userId")}
	for _, status := range r.URL.Query()["status"] {
		filter.Statuses = append(filter.Statuses, domain.ConversationStatus(status))
	}
	conversations, err := s.Conversations(r.Context(), filter)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, conversations)
}

func (s *Service) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.Conversation(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, conversation)
}

func (s *Service) handleApplyTurn(w http.ResponseWriter, r *http.Request) {
	var turn Turn
	if err := httpx.DecodeJSON(r, &turn); err != nil {
		httpx.WriteError(w, err)
		return
	}
	conversation, err := s.ApplyTurn(r.Context(), chi.URLParam(r, "id"), turn)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, conversation)
}

type assignInput struct {
	AgentName string `json:"agentName"`
}

func (s *Service) handleAssign(w http.ResponseWriter, r *http.Request) {
	var input assignInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, err)
		return
	}
	if input.AgentName == "" {
		httpx.WriteError(w, httpx.BadRequest("Informe o atendente responsável."))
		return
	}
	conversation, err := s.Assign(r.Context(), chi.URLParam(r, "id"), input.AgentName)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, conversation)
}

type agentReplyInput struct {
	AgentName string `json:"agentName"`
	Text      string `json:"text"`
}

func (s *Service) handleAgentReply(w http.ResponseWriter, r *http.Request) {
	var input agentReplyInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, err)
		return
	}
	conversation, err := s.AgentReply(r.Context(), chi.URLParam(r, "id"), input.AgentName, input.Text)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, conversation)
}

func (s *Service) handleResolve(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.Resolve(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, conversation)
}

func (s *Service) handleMarkRead(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.MarkRead(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, conversation)
}

func (s *Service) handleReset(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.Reset(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, conversation)
}

func (s *Service) handleOpenTicket(w http.ResponseWriter, r *http.Request) {
	var input TicketInput
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.WriteError(w, err)
		return
	}
	ticket, err := s.OpenTicket(r.Context(), input)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, ticket)
}

func (s *Service) handleListTickets(w http.ResponseWriter, r *http.Request) {
	tickets, err := s.Tickets(r.Context(), r.URL.Query().Get("userId"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, tickets)
}

func (s *Service) handleGetTicket(w http.ResponseWriter, r *http.Request) {
	ticket, err := s.Ticket(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ticket)
}

func (s *Service) handleUpdateTicket(w http.ResponseWriter, r *http.Request) {
	var update TicketUpdate
	if err := httpx.DecodeJSON(r, &update); err != nil {
		httpx.WriteError(w, err)
		return
	}
	ticket, err := s.UpdateTicket(r.Context(), chi.URLParam(r, "id"), update)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ticket)
}

func (s *Service) handlePurgeUser(w http.ResponseWriter, r *http.Request) {
	if err := s.PurgeUser(r.Context(), chi.URLParam(r, "userId")); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "purged"})
}
