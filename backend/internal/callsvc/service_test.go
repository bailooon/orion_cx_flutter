package callsvc

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/platform/bus"
)

func newTestService(t *testing.T) (*Service, context.Context) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(NewMemoryRepository(), bus.NewInProcess(logger), logger), context.Background()
}

func openTestConversation(t *testing.T, service *Service, ctx context.Context) domain.Conversation {
	t.Helper()
	conversation, err := service.OpenConversation(ctx, NewConversationInput{
		ID:           "CX-TEST-0001",
		UserID:       "USR-1",
		CustomerName: "Cliente Demo",
		PlanName:     "Claro Fibra 500 Mega",
	})
	if err != nil {
		t.Fatalf("OpenConversation: %v", err)
	}
	return conversation
}

func TestApplyTurnAppendsInOrderAndUpdatesState(t *testing.T) {
	service, ctx := newTestService(t)
	conversation := openTestConversation(t, service, ctx)

	intent := "SUPORTE_TECNICO"
	confidence := 0.94
	updated, err := service.ApplyTurn(ctx, conversation.ID, Turn{
		Messages: []MessageInput{
			{Actor: domain.ActorCustomer, Text: "minha internet está lenta", Channel: domain.ChannelWhatsApp},
			{Actor: domain.ActorAssistant, Text: "Posso reiniciar o sinal?", Channel: domain.ChannelWhatsApp},
		},
		Intent:           &intent,
		IntentConfidence: &confidence,
		SetPendingAction: true,
		PendingAction:    domain.ActionRestartSignal,
	})
	if err != nil {
		t.Fatalf("ApplyTurn: %v", err)
	}

	if len(updated.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(updated.Messages))
	}
	if updated.Messages[0].Actor != domain.ActorCustomer || updated.Messages[1].Actor != domain.ActorAssistant {
		t.Fatal("messages of one turn must keep their order")
	}
	if updated.Intent != intent || updated.IntentConfidence != confidence {
		t.Fatalf("classification not persisted: %+v", updated)
	}
	if updated.PendingAction == nil || *updated.PendingAction != domain.ActionRestartSignal {
		t.Fatalf("pending action not persisted: %v", updated.PendingAction)
	}
}

// TestPendingActionClearIsExplicit pins the semantics that distinguish "leave
// the pending action alone" from "clear it".
func TestPendingActionClearIsExplicit(t *testing.T) {
	service, ctx := newTestService(t)
	conversation := openTestConversation(t, service, ctx)

	if _, err := service.ApplyTurn(ctx, conversation.ID, Turn{
		Messages:         []MessageInput{{Actor: domain.ActorAssistant, Text: "Posso reiniciar?", Channel: domain.ChannelApp}},
		SetPendingAction: true,
		PendingAction:    domain.ActionRestartSignal,
	}); err != nil {
		t.Fatalf("ApplyTurn: %v", err)
	}

	// A turn that does not set the flag must not touch the pending action.
	untouched, err := service.ApplyTurn(ctx, conversation.ID, Turn{
		Messages: []MessageInput{{Actor: domain.ActorCustomer, Text: "só um momento", Channel: domain.ChannelApp}},
	})
	if err != nil {
		t.Fatalf("ApplyTurn: %v", err)
	}
	if untouched.PendingAction == nil {
		t.Fatal("a turn without SetPendingAction must preserve the pending action")
	}

	cleared, err := service.ApplyTurn(ctx, conversation.ID, Turn{
		Messages:         []MessageInput{{Actor: domain.ActorAssistant, Text: "Cancelado.", Channel: domain.ChannelApp}},
		SetPendingAction: true,
		PendingAction:    "",
	})
	if err != nil {
		t.Fatalf("ApplyTurn: %v", err)
	}
	if cleared.PendingAction != nil {
		t.Fatalf("expected the pending action to be cleared, got %v", *cleared.PendingAction)
	}
}

func TestApplyTurnRejectsInvalidMessages(t *testing.T) {
	service, ctx := newTestService(t)
	conversation := openTestConversation(t, service, ctx)

	if _, err := service.ApplyTurn(ctx, conversation.ID, Turn{
		Messages: []MessageInput{{Actor: domain.ActorCustomer, Text: "   ", Channel: domain.ChannelApp}},
	}); err == nil {
		t.Fatal("an empty message must be rejected")
	}
	if _, err := service.ApplyTurn(ctx, conversation.ID, Turn{
		Messages: []MessageInput{{Actor: domain.ActorCustomer, Text: "oi", Channel: "telegram"}},
	}); err == nil {
		t.Fatal("an unknown channel must be rejected")
	}
}

func TestAssignRequiresAQueuedConversation(t *testing.T) {
	service, ctx := newTestService(t)
	conversation := openTestConversation(t, service, ctx)

	if _, err := service.Assign(ctx, conversation.ID, "Camila Rocha"); err == nil {
		t.Fatal("an automated conversation must not be assignable")
	}

	status := domain.StatusWaitingHuman
	if _, err := service.ApplyTurn(ctx, conversation.ID, Turn{
		Messages: []MessageInput{{Actor: domain.ActorSystem, Text: "REQUIRED_HUMAN_ASSISTANCE", Channel: domain.ChannelApp}},
		Status:   &status,
	}); err != nil {
		t.Fatalf("ApplyTurn: %v", err)
	}

	assigned, err := service.Assign(ctx, conversation.ID, "Camila Rocha")
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if assigned.Status != domain.StatusInProgress {
		t.Fatalf("expected inProgress, got %s", assigned.Status)
	}
	if assigned.AssignedAgent == nil || *assigned.AssignedAgent != "Camila Rocha" {
		t.Fatalf("expected the agent to be recorded, got %v", assigned.AssignedAgent)
	}
	if assigned.HasUnreadEvent {
		t.Fatal("taking a case must clear its unread badge")
	}
}

func TestAgentReplyRequiresOwnership(t *testing.T) {
	service, ctx := newTestService(t)
	conversation := openTestConversation(t, service, ctx)

	if _, err := service.AgentReply(ctx, conversation.ID, "Camila Rocha", "olá"); err == nil {
		t.Fatal("an agent must take the case before answering")
	}
}

// TestTicketFollowsConversationStatus covers the rule that a protocol cannot
// stay open after the conversation that produced it was resolved.
func TestTicketFollowsConversationStatus(t *testing.T) {
	service, ctx := newTestService(t)
	conversation := openTestConversation(t, service, ctx)

	ticket, err := service.OpenTicket(ctx, TicketInput{
		UserID:         conversation.UserID,
		ConversationID: conversation.ID,
		Title:          "Diagnóstico de conexão",
		Category:       "SUPORTE_TECNICO",
		Channel:        domain.ChannelWhatsApp,
	})
	if err != nil {
		t.Fatalf("OpenTicket: %v", err)
	}
	if ticket.Status != domain.TicketOpen {
		t.Fatalf("expected a new ticket to be open, got %s", ticket.Status)
	}

	status := domain.StatusResolved
	if _, err := service.ApplyTurn(ctx, conversation.ID, Turn{
		Messages: []MessageInput{{Actor: domain.ActorAssistant, Text: "Sinal reiniciado.", Channel: domain.ChannelWeb}},
		Status:   &status,
	}); err != nil {
		t.Fatalf("ApplyTurn: %v", err)
	}

	reloaded, err := service.Ticket(ctx, ticket.ID)
	if err != nil {
		t.Fatalf("Ticket: %v", err)
	}
	if reloaded.Status != domain.TicketResolved {
		t.Fatalf("expected the ticket to follow the conversation to resolved, got %s", reloaded.Status)
	}
	if len(reloaded.Timeline) == 0 {
		t.Fatal("expected the status change to be recorded on the timeline")
	}
}

func TestPurgeUserRemovesEverything(t *testing.T) {
	service, ctx := newTestService(t)
	conversation := openTestConversation(t, service, ctx)
	if _, err := service.OpenTicket(ctx, TicketInput{
		UserID:         conversation.UserID,
		ConversationID: conversation.ID,
		Title:          "Chamado",
		Category:       "SUPORTE_TECNICO",
		Channel:        domain.ChannelApp,
	}); err != nil {
		t.Fatalf("OpenTicket: %v", err)
	}

	if err := service.PurgeUser(ctx, conversation.UserID); err != nil {
		t.Fatalf("PurgeUser: %v", err)
	}
	if _, err := service.Conversation(ctx, conversation.ID); err == nil {
		t.Fatal("expected the conversation to be gone")
	}
	tickets, err := service.Tickets(ctx, conversation.UserID)
	if err != nil {
		t.Fatalf("Tickets: %v", err)
	}
	if len(tickets) != 0 {
		t.Fatalf("expected no tickets after erasure, got %d", len(tickets))
	}
}

// TestConcurrentTurnsDoNotLoseMessages exercises the locking in the repository:
// two channels writing at once must not drop a message.
func TestConcurrentTurnsDoNotLoseMessages(t *testing.T) {
	service, ctx := newTestService(t)
	conversation := openTestConversation(t, service, ctx)

	const writers = 20
	var waitGroup sync.WaitGroup
	waitGroup.Add(writers)
	for index := 0; index < writers; index++ {
		go func() {
			defer waitGroup.Done()
			_, _ = service.ApplyTurn(ctx, conversation.ID, Turn{
				Messages: []MessageInput{{Actor: domain.ActorCustomer, Text: "mensagem", Channel: domain.ChannelApp}},
			})
		}()
	}
	waitGroup.Wait()

	reloaded, err := service.Conversation(ctx, conversation.ID)
	if err != nil {
		t.Fatalf("Conversation: %v", err)
	}
	if len(reloaded.Messages) != writers {
		t.Fatalf("expected %d messages, got %d", writers, len(reloaded.Messages))
	}
}

// TestReturnedConversationIsACopy guards against a caller mutating stored state
// through the slice it received.
func TestReturnedConversationIsACopy(t *testing.T) {
	service, ctx := newTestService(t)
	conversation := openTestConversation(t, service, ctx)
	if _, err := service.ApplyTurn(ctx, conversation.ID, Turn{
		Messages: []MessageInput{{Actor: domain.ActorCustomer, Text: "original", Channel: domain.ChannelApp}},
	}); err != nil {
		t.Fatalf("ApplyTurn: %v", err)
	}

	first, err := service.Conversation(ctx, conversation.ID)
	if err != nil {
		t.Fatalf("Conversation: %v", err)
	}
	first.Messages[0].Text = "adulterado"

	second, err := service.Conversation(ctx, conversation.ID)
	if err != nil {
		t.Fatalf("Conversation: %v", err)
	}
	if second.Messages[0].Text != "original" {
		t.Fatal("stored state must not be mutable through a returned copy")
	}
}
