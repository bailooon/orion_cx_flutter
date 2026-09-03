package callsvc

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/platform/bus"
	"github.com/orion-cx/orion-backend/internal/platform/httpx"
)

// Defaults applied to a brand new or reset conversation.
const (
	IntentUnclassified = "NAO_CLASSIFICADA"
	SummaryNewSession  = "Nova sessão criada. Aguardando a primeira solicitação do cliente."
)

// Service exposes the Call Management use cases.
type Service struct {
	repo   Repository
	bus    bus.Bus
	logger *slog.Logger
}

// NewService wires Call Management.
func NewService(repo Repository, eventBus bus.Bus, logger *slog.Logger) *Service {
	return &Service{repo: repo, bus: eventBus, logger: logger}
}

// NewConversationInput opens a conversation for a customer.
type NewConversationInput struct {
	ID               string `json:"id"`
	UserID           string `json:"userId"`
	CustomerName     string `json:"customerName"`
	CustomerDocument string `json:"customerDocument"`
	PlanName         string `json:"planName"`
}

// OpenConversation creates a conversation. The id is caller-supplied so the
// gateway can mint the customer-facing protocol number.
func (s *Service) OpenConversation(ctx context.Context, input NewConversationInput) (domain.Conversation, error) {
	if input.UserID == "" {
		return domain.Conversation{}, httpx.BadRequest("Informe o usuário da conversa.")
	}
	id := input.ID
	if id == "" {
		id = NewProtocolID()
	}
	now := time.Now().UTC()
	return s.repo.CreateConversation(ctx, domain.Conversation{
		ID:               id,
		UserID:           input.UserID,
		CustomerName:     input.CustomerName,
		CustomerDocument: input.CustomerDocument,
		PlanName:         input.PlanName,
		Intent:           IntentUnclassified,
		Summary:          SummaryNewSession,
		Status:           domain.StatusBot,
		CreatedAt:        now,
		UpdatedAt:        now,
		Messages:         []domain.Message{},
	})
}

// Conversation returns one conversation with its history.
func (s *Service) Conversation(ctx context.Context, id string) (domain.Conversation, error) {
	return s.repo.GetConversation(ctx, id)
}

// Conversations lists conversations matching a filter.
func (s *Service) Conversations(ctx context.Context, filter ConversationFilter) ([]domain.Conversation, error) {
	return s.repo.ListConversations(ctx, filter)
}

// ApplyTurn persists one orchestrated turn and announces the new state.
func (s *Service) ApplyTurn(ctx context.Context, id string, turn Turn) (domain.Conversation, error) {
	for _, message := range turn.Messages {
		if strings.TrimSpace(message.Text) == "" {
			return domain.Conversation{}, httpx.BadRequest("Mensagem vazia não pode ser registrada.")
		}
		if !message.Channel.Valid() {
			return domain.Conversation{}, httpx.BadRequest("Canal desconhecido: " + string(message.Channel))
		}
	}
	conversation, err := s.repo.ApplyTurn(ctx, id, turn)
	if err != nil {
		return domain.Conversation{}, err
	}
	// A turn that changes the conversation status must carry the linked tickets
	// with it, otherwise a protocol stays "open" after the conversation that
	// produced it was already resolved.
	if turn.Status != nil {
		if err := s.syncTickets(ctx, conversation, ticketStatusFor(conversation.Status),
			ticketEventFor(conversation.Status)); err != nil {
			s.logger.Warn("failed to sync tickets on turn", slog.String("err", err.Error()))
		}
	}
	s.publish(ctx, bus.EventConversationUpdated, conversation, nil)
	return conversation, nil
}

// ticketStatusFor maps a conversation status onto the ticket lifecycle.
func ticketStatusFor(status domain.ConversationStatus) domain.TicketStatus {
	switch status {
	case domain.StatusResolved:
		return domain.TicketResolved
	case domain.StatusInProgress:
		return domain.TicketInProgress
	default:
		return domain.TicketOpen
	}
}

func ticketEventFor(status domain.ConversationStatus) string {
	switch status {
	case domain.StatusResolved:
		return "Chamado concluído junto com o atendimento."
	case domain.StatusInProgress:
		return "Chamado em atendimento humano."
	case domain.StatusWaitingHuman:
		return "Chamado aguardando um atendente."
	default:
		return "Chamado em atendimento automático."
	}
}

// Reset clears a conversation, used by the demo reset button.
func (s *Service) Reset(ctx context.Context, id string) (domain.Conversation, error) {
	conversation, err := s.repo.ResetConversation(ctx, id)
	if err != nil {
		return domain.Conversation{}, err
	}
	s.publish(ctx, bus.EventConversationUpdated, conversation, nil)
	return conversation, nil
}

// Assign hands a conversation to a human agent.
//
// Two entry points lead here: the queue built by a low-confidence handoff
// (flow B, step 5), and an agent choosing to step into a conversation the
// assistant is still handling. The second case matters because an agent
// watching a live conversation go wrong should be able to intervene instead of
// waiting for the customer to get frustrated enough to trigger a handoff.
func (s *Service) Assign(ctx context.Context, id, agentName string) (domain.Conversation, error) {
	current, err := s.repo.GetConversation(ctx, id)
	if err != nil {
		return domain.Conversation{}, err
	}
	switch current.Status {
	case domain.StatusWaitingHuman, domain.StatusBot:
		// Assignable.
	case domain.StatusInProgress:
		return domain.Conversation{}, httpx.Conflict("Este atendimento já está com um atendente.")
	default:
		return domain.Conversation{}, httpx.Conflict("Este atendimento já foi concluído.")
	}

	// The customer sees a different sentence depending on where the agent came
	// from: a handoff they were promised, or an unannounced intervention.
	notice := agentName + " assumiu o atendimento."
	if current.Status == domain.StatusBot {
		notice = agentName + " entrou no atendimento e vai continuar a conversa a partir daqui."
	}

	status := domain.StatusInProgress
	unread := false
	conversation, err := s.repo.ApplyTurn(ctx, id, Turn{
		Messages: []MessageInput{{
			Actor:   domain.ActorSystem,
			Text:    notice,
			Channel: current.LastChannel(),
		}},
		Status:           &status,
		AssignedAgent:    &agentName,
		HasUnreadEvent:   &unread,
		SetPendingAction: true,
	})
	if err != nil {
		return domain.Conversation{}, err
	}

	if err := s.syncTickets(ctx, conversation, domain.TicketInProgress,
		"Atendimento assumido por "+agentName+"."); err != nil {
		s.logger.Warn("failed to sync tickets on assign", slog.String("err", err.Error()))
	}
	s.publish(ctx, bus.EventAgentAssigned, conversation, map[string]any{"agentName": agentName})
	return conversation, nil
}

// Resolve closes a conversation and its tickets.
func (s *Service) Resolve(ctx context.Context, id string) (domain.Conversation, error) {
	current, err := s.repo.GetConversation(ctx, id)
	if err != nil {
		return domain.Conversation{}, err
	}
	if current.Status == domain.StatusResolved {
		return current, nil
	}
	status := domain.StatusResolved
	unread := false
	conversation, err := s.repo.ApplyTurn(ctx, id, Turn{
		Messages: []MessageInput{{
			Actor:   domain.ActorSystem,
			Text:    "Atendimento concluído e histórico salvo.",
			Channel: current.LastChannel(),
		}},
		Status:           &status,
		HasUnreadEvent:   &unread,
		SetPendingAction: true,
	})
	if err != nil {
		return domain.Conversation{}, err
	}

	if err := s.syncTickets(ctx, conversation, domain.TicketResolved,
		"Chamado concluído."); err != nil {
		s.logger.Warn("failed to sync tickets on resolve", slog.String("err", err.Error()))
	}
	s.publish(ctx, bus.EventConversationResolved, conversation, nil)
	return conversation, nil
}

// AgentReply records a manual answer and routes it back to the channel the
// customer last used (flow B, step 6).
func (s *Service) AgentReply(ctx context.Context, id, agentName, text string) (domain.Conversation, error) {
	current, err := s.repo.GetConversation(ctx, id)
	if err != nil {
		return domain.Conversation{}, err
	}
	if current.Status != domain.StatusInProgress {
		return domain.Conversation{}, httpx.Conflict("Assuma o atendimento antes de responder.")
	}
	unread := false
	conversation, err := s.ApplyTurn(ctx, id, Turn{
		Messages:       []MessageInput{{Actor: domain.ActorAgent, Text: text, Channel: current.LastChannel()}},
		HasUnreadEvent: &unread,
	})
	if err != nil {
		return domain.Conversation{}, err
	}
	s.publish(ctx, bus.EventAgentReplied, conversation, map[string]any{
		"agentName": agentName,
		"preview":   preview(text),
	})
	return conversation, nil
}

// MarkRead clears the unread badge of a queue entry.
func (s *Service) MarkRead(ctx context.Context, id string) (domain.Conversation, error) {
	unread := false
	return s.ApplyTurn(ctx, id, Turn{HasUnreadEvent: &unread})
}

// OpenTicket creates a protocol linked to a conversation (RF006).
func (s *Service) OpenTicket(ctx context.Context, input TicketInput) (domain.Ticket, error) {
	if input.ID == "" {
		input.ID = NewTicketID()
	}
	if input.Status == "" {
		input.Status = domain.TicketOpen
	}
	ticket, err := s.repo.CreateTicket(ctx, input)
	if err != nil {
		return domain.Ticket{}, err
	}
	event := bus.NewEvent(bus.EventTicketCreated)
	event.UserID = ticket.UserID
	event.ConversationID = ticket.ConversationID
	event.TicketID = ticket.ID
	event.Channel = string(ticket.Channel)
	event.Payload = map[string]any{"title": ticket.Title, "status": string(ticket.Status)}
	_ = s.bus.Publish(ctx, event)
	return ticket, nil
}

// Tickets lists the protocols of a customer.
func (s *Service) Tickets(ctx context.Context, userID string) ([]domain.Ticket, error) {
	return s.repo.ListTickets(ctx, userID)
}

// Ticket returns a single protocol.
func (s *Service) Ticket(ctx context.Context, id string) (domain.Ticket, error) {
	return s.repo.GetTicket(ctx, id)
}

// UpdateTicket advances a protocol and notifies the customer.
func (s *Service) UpdateTicket(ctx context.Context, id string, update TicketUpdate) (domain.Ticket, error) {
	ticket, err := s.repo.UpdateTicket(ctx, id, update)
	if err != nil {
		return domain.Ticket{}, err
	}
	event := bus.NewEvent(bus.EventTicketUpdated)
	event.UserID = ticket.UserID
	event.ConversationID = ticket.ConversationID
	event.TicketID = ticket.ID
	event.Channel = string(ticket.Channel)
	event.Payload = map[string]any{"status": string(ticket.Status), "event": update.Event}
	_ = s.bus.Publish(ctx, event)
	return ticket, nil
}

// syncTickets moves every ticket of a conversation to the given status.
func (s *Service) syncTickets(ctx context.Context, conversation domain.Conversation, status domain.TicketStatus, event string) error {
	tickets, err := s.repo.TicketsByConversation(ctx, conversation.ID)
	if err != nil {
		return err
	}
	for _, ticket := range tickets {
		if ticket.Status == status {
			continue
		}
		if _, err := s.UpdateTicket(ctx, ticket.ID, TicketUpdate{Status: &status, Event: event}); err != nil {
			return err
		}
	}
	return nil
}

// PurgeUser erases a customer's operational data (LGPD).
func (s *Service) PurgeUser(ctx context.Context, userID string) error {
	return s.repo.PurgeUser(ctx, userID)
}

// CountConversations reports the table size, used by the seeder.
func (s *Service) CountConversations(ctx context.Context) (int, error) {
	return s.repo.CountConversations(ctx)
}

// publish emits a conversation-shaped event, tolerating bus failures.
func (s *Service) publish(ctx context.Context, eventType string, conversation domain.Conversation, payload map[string]any) {
	event := bus.NewEvent(eventType)
	event.UserID = conversation.UserID
	event.ConversationID = conversation.ID
	event.Channel = string(conversation.LastChannel())
	event.Payload = map[string]any{
		"status":     string(conversation.Status),
		"intent":     conversation.Intent,
		"confidence": conversation.IntentConfidence,
		"summary":    conversation.Summary,
	}
	for key, value := range payload {
		event.Payload[key] = value
	}
	if err := s.bus.Publish(ctx, event); err != nil {
		s.logger.Warn("failed to publish event", slog.String("type", eventType), slog.String("err", err.Error()))
	}
}

// NewProtocolID mints a customer-facing conversation protocol.
func NewProtocolID() string {
	return fmt.Sprintf("CX-%d-%04d", time.Now().Year(), rand.Intn(10000))
}

// NewTicketID mints a customer-facing ticket protocol.
func NewTicketID() string {
	return fmt.Sprintf("TCK-%d-%04d", time.Now().Year(), rand.Intn(10000))
}

// preview trims a message for use inside an event payload, so a full message
// body never travels on the bus.
func preview(text string) string {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) <= 60 {
		return trimmed
	}
	return trimmed[:60] + "..."
}
