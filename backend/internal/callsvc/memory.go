package callsvc

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/platform/httpx"
)

// MemoryRepository is the dependency-free implementation of Repository. It is
// what the unit tests run against and what backs the stack when no Postgres URL
// is configured.
type MemoryRepository struct {
	mu            sync.RWMutex
	conversations map[string]*domain.Conversation
	tickets       map[string]*domain.Ticket
	audit         []domain.AuditEntry
}

// NewMemoryRepository builds an empty repository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		conversations: make(map[string]*domain.Conversation),
		tickets:       make(map[string]*domain.Ticket),
	}
}

// cloneConversation returns a deep copy so callers can never mutate stored
// state through the returned slices.
func cloneConversation(source *domain.Conversation) domain.Conversation {
	copied := *source
	copied.Messages = append([]domain.Message(nil), source.Messages...)
	if source.PendingAction != nil {
		value := *source.PendingAction
		copied.PendingAction = &value
	}
	if source.AssignedAgent != nil {
		value := *source.AssignedAgent
		copied.AssignedAgent = &value
	}
	return copied
}

func cloneTicket(source *domain.Ticket) domain.Ticket {
	copied := *source
	copied.Timeline = append([]domain.TicketEvent(nil), source.Timeline...)
	return copied
}

// CreateConversation stores a new conversation.
func (r *MemoryRepository) CreateConversation(_ context.Context, conversation domain.Conversation) (domain.Conversation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.conversations[conversation.ID]; exists {
		return domain.Conversation{}, httpx.Conflict("Conversa já existe.")
	}
	stored := conversation
	if stored.Messages == nil {
		stored.Messages = []domain.Message{}
	}
	r.conversations[stored.ID] = &stored
	return cloneConversation(&stored), nil
}

// GetConversation returns one conversation with its full history.
func (r *MemoryRepository) GetConversation(_ context.Context, id string) (domain.Conversation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	stored, ok := r.conversations[id]
	if !ok {
		return domain.Conversation{}, httpx.ErrNotFound
	}
	return cloneConversation(stored), nil
}

// ListConversations returns conversations matching the filter, newest activity
// last so the agent queue shows the oldest waiting customer first.
func (r *MemoryRepository) ListConversations(_ context.Context, filter ConversationFilter) ([]domain.Conversation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	allowed := make(map[domain.ConversationStatus]struct{}, len(filter.Statuses))
	for _, status := range filter.Statuses {
		allowed[status] = struct{}{}
	}

	result := make([]domain.Conversation, 0, len(r.conversations))
	for _, stored := range r.conversations {
		if filter.UserID != "" && stored.UserID != filter.UserID {
			continue
		}
		if len(allowed) > 0 {
			if _, ok := allowed[stored.Status]; !ok {
				continue
			}
		}
		result = append(result, cloneConversation(stored))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt.Before(result[j].UpdatedAt) })
	return result, nil
}

// ApplyTurn appends messages and updates state under a single lock, which is
// the in-memory equivalent of the Postgres transaction.
func (r *MemoryRepository) ApplyTurn(_ context.Context, id string, turn Turn) (domain.Conversation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored, ok := r.conversations[id]
	if !ok {
		return domain.Conversation{}, httpx.ErrNotFound
	}

	now := time.Now().UTC()
	for index, message := range turn.Messages {
		stored.Messages = append(stored.Messages, domain.Message{
			ID:             "MSG-" + uuid.NewString()[:8],
			ConversationID: id,
			Actor:          message.Actor,
			Text:           message.Text,
			Channel:        message.Channel,
			// Nudge each timestamp forward so messages written in the same turn
			// keep their order when sorted by sent_at.
			SentAt: now.Add(time.Duration(index) * time.Millisecond),
		})
	}
	applyTurnFields(stored, turn, now)
	stored.UpdatedAt = now
	return cloneConversation(stored), nil
}

// applyTurnFields copies the optional fields of a turn onto a conversation. It
// is shared by both repositories so their semantics cannot drift.
func applyTurnFields(target *domain.Conversation, turn Turn, now time.Time) {
	if turn.Intent != nil {
		target.Intent = *turn.Intent
	}
	if turn.IntentConfidence != nil {
		target.IntentConfidence = *turn.IntentConfidence
	}
	if turn.Summary != nil {
		target.Summary = *turn.Summary
	}
	if turn.Status != nil {
		if *turn.Status == domain.StatusResolved && target.ResolvedAt == nil {
			resolvedAt := now
			target.ResolvedAt = &resolvedAt
		}
		target.Status = *turn.Status
	}
	if turn.HasUnreadEvent != nil {
		target.HasUnreadEvent = *turn.HasUnreadEvent
	}
	if turn.AssignedAgent != nil {
		value := *turn.AssignedAgent
		if value == "" {
			target.AssignedAgent = nil
		} else {
			target.AssignedAgent = &value
		}
	}
	if turn.SetPendingAction {
		if turn.PendingAction == "" {
			target.PendingAction = nil
		} else {
			value := turn.PendingAction
			target.PendingAction = &value
		}
	}
}

// ResetConversation clears the history, used by the demo reset button.
func (r *MemoryRepository) ResetConversation(_ context.Context, id string) (domain.Conversation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored, ok := r.conversations[id]
	if !ok {
		return domain.Conversation{}, httpx.ErrNotFound
	}
	stored.Messages = []domain.Message{}
	stored.Intent = IntentUnclassified
	stored.IntentConfidence = 0
	stored.Summary = SummaryNewSession
	stored.Status = domain.StatusBot
	stored.PendingAction = nil
	stored.AssignedAgent = nil
	stored.HasUnreadEvent = false
	stored.ResolvedAt = nil
	stored.SatisfactionScore = nil
	stored.SatisfactionComment = nil
	stored.RatedAt = nil
	stored.UpdatedAt = time.Now().UTC()
	return cloneConversation(stored), nil
}

// RateConversation stores the satisfaction survey.
func (r *MemoryRepository) RateConversation(_ context.Context, id string, score int, comment string) (domain.Conversation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored, ok := r.conversations[id]
	if !ok {
		return domain.Conversation{}, httpx.ErrNotFound
	}
	if stored.Status != domain.StatusResolved {
		return domain.Conversation{}, httpx.Conflict("A pesquisa só pode ser respondida depois que o atendimento for concluído.")
	}
	if stored.SatisfactionScore != nil {
		return domain.Conversation{}, httpx.Conflict("Este atendimento já foi avaliado.")
	}
	now := time.Now().UTC()
	stored.SatisfactionScore = &score
	stored.SatisfactionComment = &comment
	stored.RatedAt = &now
	return cloneConversation(stored), nil
}

// RecordAudit appends AI audit entries.
func (r *MemoryRepository) RecordAudit(_ context.Context, entries []domain.AuditEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.audit = append(r.audit, entries...)
	// Bounded like a ring buffer: the in-memory backend must not grow forever.
	if len(r.audit) > 5000 {
		r.audit = append([]domain.AuditEntry(nil), r.audit[len(r.audit)-5000:]...)
	}
	return nil
}

// ListAudit returns entries newer than since, newest first.
func (r *MemoryRepository) ListAudit(_ context.Context, since time.Time, limit int) ([]domain.AuditEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]domain.AuditEntry, 0)
	for index := len(r.audit) - 1; index >= 0; index-- {
		if r.audit[index].CreatedAt.Before(since) {
			continue
		}
		result = append(result, r.audit[index])
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}

// CountConversations reports the table size, used by the seeder.
func (r *MemoryRepository) CountConversations(context.Context) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.conversations), nil
}

// CreateTicket opens a ticket with its first timeline entry.
func (r *MemoryRepository) CreateTicket(_ context.Context, input TicketInput) (domain.Ticket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	ticket := domain.Ticket{
		ID:             input.ID,
		UserID:         input.UserID,
		ConversationID: input.ConversationID,
		Title:          input.Title,
		Category:       input.Category,
		Status:         input.Status,
		Channel:        input.Channel,
		CreatedAt:      now,
		UpdatedAt:      now,
		Timeline:       []domain.TicketEvent{},
	}
	if input.FirstEvent != "" {
		ticket.Timeline = append(ticket.Timeline, domain.TicketEvent{At: now, Description: input.FirstEvent})
	}
	r.tickets[ticket.ID] = &ticket
	return cloneTicket(&ticket), nil
}

// GetTicket returns one ticket.
func (r *MemoryRepository) GetTicket(_ context.Context, id string) (domain.Ticket, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	stored, ok := r.tickets[id]
	if !ok {
		return domain.Ticket{}, httpx.ErrNotFound
	}
	return cloneTicket(stored), nil
}

// ListTickets returns a customer's protocols, newest first.
func (r *MemoryRepository) ListTickets(_ context.Context, userID string) ([]domain.Ticket, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]domain.Ticket, 0)
	for _, stored := range r.tickets {
		if userID == "" || stored.UserID == userID {
			result = append(result, cloneTicket(stored))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

// TicketsByConversation returns the protocols linked to a conversation.
func (r *MemoryRepository) TicketsByConversation(_ context.Context, conversationID string) ([]domain.Ticket, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]domain.Ticket, 0)
	for _, stored := range r.tickets {
		if stored.ConversationID == conversationID {
			result = append(result, cloneTicket(stored))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

// UpdateTicket advances the status and appends a timeline entry.
func (r *MemoryRepository) UpdateTicket(_ context.Context, id string, update TicketUpdate) (domain.Ticket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored, ok := r.tickets[id]
	if !ok {
		return domain.Ticket{}, httpx.ErrNotFound
	}
	now := time.Now().UTC()
	if update.Status != nil {
		stored.Status = *update.Status
	}
	if update.Event != "" {
		stored.Timeline = append(stored.Timeline, domain.TicketEvent{At: now, Description: update.Event})
	}
	stored.UpdatedAt = now
	return cloneTicket(stored), nil
}

// PurgeUser removes every trace of a user from Call Management.
func (r *MemoryRepository) PurgeUser(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, conversation := range r.conversations {
		if conversation.UserID == userID {
			delete(r.conversations, id)
		}
	}
	for id, ticket := range r.tickets {
		if ticket.UserID == userID {
			delete(r.tickets, id)
		}
	}
	return nil
}

// Ping always succeeds: the store lives in this process.
func (r *MemoryRepository) Ping(context.Context) error { return nil }
