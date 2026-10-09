package gatewaysvc

import (
	"context"
	"log/slog"
	"time"

	"github.com/orion-cx/orion-backend/internal/agents"
	"github.com/orion-cx/orion-backend/internal/callsvc"
	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/platform/httpx"
	"github.com/orion-cx/orion-backend/internal/platform/security"
)

// pendingCall is a sensitive tool call waiting for the customer's "yes".
type pendingCall struct {
	Tool string
	Args map[string]string
}

// confirmationResult is the outcome of the double check on a pending action.
type confirmationResult struct {
	reply       string
	resolves    bool
	ticketTitle string
	rejected    bool
}

// record writes AI audit entries. Audit is evidence, so it is written
// synchronously; a failure is logged but never blocks the customer.
func (s *Service) record(ctx context.Context, entries ...domain.AuditEntry) {
	if len(entries) == 0 {
		return
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := s.calls.RecordAudit(writeCtx, entries); err != nil {
		s.logger.Warn("failed to record AI audit", slog.String("err", err.Error()))
	}
}

func auditEntry(kind string, principal security.Principal, conversationID, agent, tool, status, detail string) domain.AuditEntry {
	return domain.AuditEntry{
		Kind: kind, UserID: principal.UserID, ConversationID: conversationID,
		Agent: agent, Tool: tool, Status: status, Detail: detail,
	}
}

// auditOutcome turns an agent run into audit entries.
func auditOutcome(principal security.Principal, conversationID string, outcome agents.Outcome) []domain.AuditEntry {
	entries := make([]domain.AuditEntry, 0, len(outcome.Executions)+1)
	for _, execution := range outcome.Executions {
		kind := domain.AuditToolCall
		if execution.Status == agents.StatusConfirmationRequested {
			kind = domain.AuditConfirmRequested
		}
		entry := auditEntry(kind, principal, conversationID, outcome.Agent.Name, execution.Tool, execution.Status,
			execution.Detail+" · motor: "+outcome.Engine)
		entry.LatencyMs = execution.LatencyMs
		entries = append(entries, entry)
	}
	if outcome.ReplyCorrection != "" {
		entries = append(entries, auditEntry(domain.AuditReplyCorrected, principal, conversationID,
			outcome.Agent.Name, "", "corrigida", outcome.ReplyCorrection))
	}
	return entries
}

// applyAgent runs the agent that owns the turn and folds its outcome into the
// routing decision. On any failure the decision keeps its fallback reply, so
// the customer is always answered.
func (s *Service) applyAgent(ctx context.Context, principal security.Principal, conversation domain.Conversation,
	text string, channel domain.Channel, intent string, decision *route) *pendingCall {

	outcome, err := s.agents.Handle(ctx, agents.Request{
		Principal:      principal,
		ConversationID: conversation.ID,
		Intent:         intent,
		Text:           text,
		Channel:        channel,
		History:        conversation.Messages,
	})
	if err != nil {
		s.logger.Warn("agent failed, using routing fallback", slog.String("err", err.Error()))
		return fallbackPending(decision)
	}
	if outcome.Shed {
		// Overload: answer with the routing fallback (no tools, no model) and
		// leave evidence on the dashboard.
		s.record(ctx, auditEntry(domain.AuditQueueShed, principal, conversation.ID, outcome.Agent.Name, "",
			"descartada", "fila de agentes saturada: resposta de contingência"))
		return fallbackPending(decision)
	}

	s.record(ctx, auditOutcome(principal, conversation.ID, outcome)...)

	if outcome.Handoff {
		decision.handoff = true
		decision.reason = outcome.HandoffReason
		decision.reply = outcome.Reply
		decision.pendingAction = ""
		decision.resolves = false
		decision.ticketTitle = "Atendimento encaminhado pelo " + outcome.Agent.Name
		return nil
	}

	decision.reply = outcome.Reply
	decision.pendingAction = outcome.PendingAction
	decision.resolves = outcome.Resolves
	if outcome.TicketTitle != "" {
		decision.ticketTitle = outcome.TicketTitle
	}
	if outcome.PendingTool != "" {
		return &pendingCall{Tool: outcome.PendingTool, Args: outcome.PendingArgs}
	}
	return nil
}

// fallbackPending keeps the legacy restart question confirmable when the
// agent could not run.
func fallbackPending(decision *route) *pendingCall {
	if tool := agents.PendingToolFor(decision.pendingAction); tool != "" {
		return &pendingCall{Tool: tool}
	}
	return nil
}

// confirmPending is the double check. The customer's "yes" is necessary but not
// sufficient: before a sensitive tool runs, the gateway re-verifies that
//   - the caller owns the conversation and it is still automated,
//   - the conversation is really waiting for a confirmation,
//   - the stored tool matches the pending action (no swapped request),
//   - the confirmation has not expired,
//
// and the tool re-validates its own arguments at execution time.
func (s *Service) confirmPending(ctx context.Context, principal security.Principal, conversation domain.Conversation) confirmationResult {
	reject := func(detail string) confirmationResult {
		s.record(ctx, auditEntry(domain.AuditConfirmRejected, principal, conversation.ID, "", "", "recusada", detail))
		return confirmationResult{
			rejected: true,
			reply: "Por segurança, não executei a ação: a confirmação não pôde ser validada (" + detail +
				"). Me diga de novo o que você precisa e eu refaço a verificação.",
		}
	}

	if conversation.UserID != principal.UserID {
		return reject("conversa de outro cliente")
	}
	if conversation.Status != domain.StatusBot {
		return reject("atendimento não está mais com o assistente")
	}
	pendingAction := ""
	if conversation.PendingAction != nil {
		pendingAction = *conversation.PendingAction
	}
	if !agents.IsConfirmationAction(pendingAction) {
		return reject("nenhuma ação aguardando confirmação")
	}

	expected := agents.PendingToolFor(pendingAction)
	call := pendingCall{Tool: expected}
	session, found, _ := s.sessions.Get(ctx, principal.UserID)
	if found && session.ConversationID == conversation.ID && session.PendingTool != "" {
		if session.PendingTool != expected {
			return reject("ação pendente divergente")
		}
		if !session.PendingSince.IsZero() && time.Since(session.PendingSince) > s.cfg.ConfirmationTTL {
			return reject("confirmação expirada")
		}
		call = pendingCall{Tool: session.PendingTool, Args: session.PendingArgs}
	} else if expected != agents.ToolRestartSignal {
		// Only the argument-free restart can be rebuilt from the conversation
		// alone; anything with arguments needs the stored request.
		return reject("contexto da confirmação indisponível")
	}

	outcome, err := s.agents.ExecuteConfirmed(ctx, principal, conversation.ID, call.Tool, call.Args)
	s.record(ctx, auditOutcome(principal, conversation.ID, outcome)...)
	if err != nil {
		return reject("verificação da ação falhou")
	}
	s.record(ctx, auditEntry(domain.AuditConfirmAccepted, principal, conversation.ID, outcome.Agent.Name, call.Tool, "executada", ""))
	return confirmationResult{reply: outcome.Reply, resolves: outcome.Resolves, ticketTitle: outcome.TicketTitle}
}

// storeSession saves the resume context. When pending is nil and the
// conversation still waits for a confirmation, the stored pending call is
// kept: a channel switch must not lose what the customer is confirming.
func (s *Service) storeSession(ctx context.Context, userID string, conversation domain.Conversation,
	channel domain.Channel, intent string, unclearTurns int, pending *pendingCall) {

	pendingAction := ""
	if conversation.PendingAction != nil {
		pendingAction = *conversation.PendingAction
	}
	session := domain.SessionContext{
		SessionID:      "SES-" + conversation.ID,
		UserID:         userID,
		ConversationID: conversation.ID,
		LastChannel:    channel,
		PendingAction:  pendingAction,
		LastIntent:     intent,
		UnclearTurns:   unclearTurns,
	}
	switch {
	case pending != nil:
		session.PendingTool = pending.Tool
		session.PendingArgs = pending.Args
		session.PendingSince = time.Now().UTC()
	case agents.IsConfirmationAction(pendingAction):
		if previous, found, _ := s.sessions.Get(ctx, userID); found && previous.ConversationID == conversation.ID {
			session.PendingTool = previous.PendingTool
			session.PendingArgs = previous.PendingArgs
			session.PendingSince = previous.PendingSince
		}
	}
	if err := s.sessions.Save(ctx, session); err != nil {
		// The journey still works without the cache: ActiveConversation falls
		// back to Call Management. Losing the context only costs a lookup.
		s.logger.Warn("failed to persist session context", slog.String("err", err.Error()))
	}
}

// openFreshConversation starts a new conversation for a customer whose last
// one is closed, so a resolved (and possibly rated) attendance is never
// reopened by the next message.
func (s *Service) openFreshConversation(ctx context.Context, principal security.Principal, channel domain.Channel) (domain.Conversation, error) {
	profile, err := s.auth.Profile(ctx, principal.UserID)
	if err != nil {
		return domain.Conversation{}, err
	}
	conversation, err := s.calls.Open(ctx, callsvc.NewConversationInput{
		UserID:           profile.ID,
		CustomerName:     profile.Name,
		CustomerDocument: profile.DocumentMask,
		PlanName:         profile.PlanName,
	})
	if err != nil {
		return domain.Conversation{}, err
	}
	s.storeSession(ctx, principal.UserID, conversation, channel, "", 0, nil)
	return conversation, nil
}

// SubmitSurvey records the customer's satisfaction answer.
func (s *Service) SubmitSurvey(ctx context.Context, principal security.Principal, conversationID string, score int, comment string) (domain.Conversation, error) {
	conversation, err := s.calls.Get(ctx, conversationID)
	if err != nil {
		return domain.Conversation{}, err
	}
	if conversation.UserID != principal.UserID {
		// Only the customer who was served may rate the service.
		return domain.Conversation{}, httpx.ErrForbidden
	}
	rated, err := s.calls.Rate(ctx, conversationID, score, comment)
	if err != nil {
		return domain.Conversation{}, err
	}
	s.broadcastSnapshots(ctx)
	return rated, nil
}

// Dashboard is the payload of the agent dashboards.
type Dashboard struct {
	Analytics callsvc.Analytics `json:"analytics"`
	Queues    struct {
		Agents any `json:"agents"`
		NLU    any `json:"nlu"`
	} `json:"queues"`
	Agents   []agents.Agent `json:"agents"`
	Engine   string         `json:"engine"`
	Security struct {
		RateLimitPerMinute  int     `json:"rateLimitPerMinute"`
		MaxInputChars       int     `json:"maxInputChars"`
		MaxToolCallsPerTurn int     `json:"maxToolCallsPerTurn"`
		ConfirmationTTLMin  float64 `json:"confirmationTtlMinutes"`
		ConfidenceThreshold float64 `json:"confidenceThreshold"`
	} `json:"security"`
}

// Dashboard aggregates analytics, live queue state and the security posture.
func (s *Service) Dashboard(ctx context.Context) (Dashboard, error) {
	var dashboard Dashboard
	analytics, err := s.calls.Analytics(ctx)
	if err != nil {
		return dashboard, err
	}
	dashboard.Analytics = analytics
	dashboard.Queues.Agents = s.agents.QueueStats()
	if stats, err := s.nlp.Stats(ctx); err == nil {
		dashboard.Queues.NLU = stats
	}
	dashboard.Agents = agents.AllAgents
	dashboard.Engine = agents.EngineDeterministic
	if s.agents.UsesLLM() {
		dashboard.Engine = agents.EngineClaude
	}
	dashboard.Security.RateLimitPerMinute = s.cfg.RateLimitPerMinute
	dashboard.Security.MaxInputChars = agents.MaxInputRunes
	dashboard.Security.MaxToolCallsPerTurn = agents.MaxToolCallsPerTurn
	dashboard.Security.ConfirmationTTLMin = s.cfg.ConfirmationTTL.Minutes()
	dashboard.Security.ConfidenceThreshold = s.cfg.ConfidenceThreshold
	return dashboard, nil
}
