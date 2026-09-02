// Package callsvc is the ORION Call Management service. It owns the
// conversation history (RF002) and the ticket lifecycle (RF006).
package callsvc

import (
	"context"

	"github.com/orion-cx/orion-backend/internal/domain"
)

// MessageInput is one message to append during a turn.
type MessageInput struct {
	Actor   domain.Actor   `json:"actor"`
	Text    string         `json:"text"`
	Channel domain.Channel `json:"channel"`
}

// Turn is the atomic unit the gateway writes after orchestrating one customer
// interaction: the messages produced and the new classification/state of the
// conversation, applied in a single transaction so the history and the status
// can never disagree.
type Turn struct {
	Messages []MessageInput `json:"messages"`

	Intent           *string                    `json:"intent,omitempty"`
	IntentConfidence *float64                   `json:"intentConfidence,omitempty"`
	Summary          *string                    `json:"summary,omitempty"`
	Status           *domain.ConversationStatus `json:"status,omitempty"`
	AssignedAgent    *string                    `json:"assignedAgent,omitempty"`
	HasUnreadEvent   *bool                      `json:"hasUnreadEvent,omitempty"`

	// PendingAction is only written when SetPendingAction is true, which lets
	// the caller distinguish "leave it alone" from "clear it".
	PendingAction    string `json:"pendingAction,omitempty"`
	SetPendingAction bool   `json:"setPendingAction,omitempty"`
}

// ConversationFilter narrows a conversation listing.
type ConversationFilter struct {
	UserID   string
	Statuses []domain.ConversationStatus
}

// TicketInput creates a ticket.
type TicketInput struct {
	ID             string              `json:"id"`
	UserID         string              `json:"userId"`
	ConversationID string              `json:"conversationId"`
	Title          string              `json:"title"`
	Category       string              `json:"category"`
	Status         domain.TicketStatus `json:"status"`
	Channel        domain.Channel      `json:"channel"`
	FirstEvent     string              `json:"firstEvent"`
}

// TicketUpdate advances a ticket and appends a timeline entry.
type TicketUpdate struct {
	Status *domain.TicketStatus `json:"status,omitempty"`
	Event  string               `json:"event,omitempty"`
}

// Repository is the persistence contract of Call Management.
type Repository interface {
	CreateConversation(ctx context.Context, conversation domain.Conversation) (domain.Conversation, error)
	GetConversation(ctx context.Context, id string) (domain.Conversation, error)
	ListConversations(ctx context.Context, filter ConversationFilter) ([]domain.Conversation, error)
	ApplyTurn(ctx context.Context, id string, turn Turn) (domain.Conversation, error)
	ResetConversation(ctx context.Context, id string) (domain.Conversation, error)
	CountConversations(ctx context.Context) (int, error)

	CreateTicket(ctx context.Context, input TicketInput) (domain.Ticket, error)
	GetTicket(ctx context.Context, id string) (domain.Ticket, error)
	ListTickets(ctx context.Context, userID string) ([]domain.Ticket, error)
	TicketsByConversation(ctx context.Context, conversationID string) ([]domain.Ticket, error)
	UpdateTicket(ctx context.Context, id string, update TicketUpdate) (domain.Ticket, error)

	// PurgeUser deletes every conversation, message and ticket of a user, used
	// by the LGPD erasure endpoint.
	PurgeUser(ctx context.Context, userID string) error
	Ping(ctx context.Context) error
}
