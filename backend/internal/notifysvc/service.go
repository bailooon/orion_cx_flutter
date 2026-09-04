package notifysvc

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/platform/bus"
	"github.com/orion-cx/orion-backend/internal/platform/httpx"
)

// consumerGroup identifies this service on the event bus.
const consumerGroup = "orion-notification"

// Service turns bus events into notifications.
type Service struct {
	repo   Repository
	bus    bus.Bus
	logger *slog.Logger
}

// NewService wires the Notification service.
func NewService(repo Repository, eventBus bus.Bus, logger *slog.Logger) *Service {
	return &Service{repo: repo, bus: eventBus, logger: logger}
}

// Start subscribes to the bus. It returns as soon as the subscription is
// registered; consumption runs in the background.
func (s *Service) Start(ctx context.Context) error {
	return s.bus.Subscribe(ctx, consumerGroup, s.handleEvent)
}

// handleEvent maps a domain event to a customer-facing notification. Events
// that are not customer-relevant are ignored.
func (s *Service) handleEvent(ctx context.Context, event bus.Event) {
	if event.UserID == "" {
		return
	}

	var title, body string
	switch event.Type {
	case bus.EventHumanAssistanceRequired:
		title = "Atendimento transferido para um especialista"
		body = "Sua solicitação foi encaminhada para um atendente humano. Todo o histórico foi enviado junto, você não precisará repetir as informações."
	case bus.EventAgentAssigned:
		agent, _ := event.Payload["agentName"].(string)
		title = "Um atendente assumiu seu caso"
		body = agent + " está cuidando do seu atendimento agora."
		if agent == "" {
			body = "Um atendente está cuidando do seu atendimento agora."
		}
	case bus.EventAgentReplied:
		title = "Nova resposta no seu atendimento"
		body = "Você recebeu uma resposta do atendente no protocolo " + event.ConversationID + "."
	case bus.EventConversationResolved:
		title = "Atendimento concluído"
		body = "O protocolo " + event.ConversationID + " foi finalizado. O histórico continua disponível no seu portal."
	case bus.EventTicketCreated:
		ticketTitle, _ := event.Payload["title"].(string)
		title = "Chamado " + event.TicketID + " aberto"
		body = ticketTitle
	case bus.EventTicketUpdated:
		status, _ := event.Payload["status"].(string)
		title = "Chamado " + event.TicketID + " atualizado"
		body = "Novo status: " + statusLabel(status) + "."
	default:
		// CONVERSATION_UPDATED is high-frequency internal state; notifying on
		// every turn would spam the customer.
		return
	}

	channel := domain.Channel(event.Channel)
	if !channel.Valid() {
		channel = domain.ChannelApp
	}

	notification := domain.Notification{
		ID:        "NTF-" + uuid.NewString()[:8],
		UserID:    event.UserID,
		Title:     title,
		Body:      body,
		Channel:   channel,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.repo.Create(ctx, notification); err != nil {
		s.logger.Error("failed to persist notification",
			slog.String("event_type", event.Type),
			slog.String("err", err.Error()),
		)
		return
	}
	// Stands in for the Amazon SNS publish of the production design.
	s.logger.Info("notification dispatched",
		slog.String("notification_id", notification.ID),
		slog.String("event_type", event.Type),
		slog.String("channel", string(notification.Channel)),
	)
}

func statusLabel(status string) string {
	switch domain.TicketStatus(status) {
	case domain.TicketOpen:
		return "aberto"
	case domain.TicketInProgress:
		return "em atendimento"
	case domain.TicketResolved:
		return "concluído"
	default:
		return status
	}
}

// List returns a customer's notifications.
func (s *Service) List(ctx context.Context, userID string) ([]domain.Notification, error) {
	return s.repo.ListByUser(ctx, userID)
}

// Routes mounts the internal API of the Notification service.
func (s *Service) Routes() http.Handler {
	router := chi.NewRouter()

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := s.repo.Ping(r.Context()); err != nil {
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded"})
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "orion-notification"})
	})

	router.Get("/v1/notifications", func(w http.ResponseWriter, r *http.Request) {
		notifications, err := s.List(r.Context(), r.URL.Query().Get("userId"))
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, notifications)
	})

	router.Post("/v1/notifications/{id}/read", func(w http.ResponseWriter, r *http.Request) {
		if err := s.repo.MarkRead(r.Context(), chi.URLParam(r, "id")); err != nil {
			httpx.WriteError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "read"})
	})

	router.Post("/v1/notifications/read-all", func(w http.ResponseWriter, r *http.Request) {
		if err := s.repo.MarkAllRead(r.Context(), r.URL.Query().Get("userId")); err != nil {
			httpx.WriteError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "read"})
	})

	router.Delete("/v1/users/{userId}/notifications", func(w http.ResponseWriter, r *http.Request) {
		if err := s.repo.DeleteByUser(r.Context(), chi.URLParam(r, "userId")); err != nil {
			httpx.WriteError(w, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	})

	return router
}
