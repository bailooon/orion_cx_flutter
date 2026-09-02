package gatewaysvc

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/orion-cx/orion-backend/internal/callsvc"
	"github.com/orion-cx/orion-backend/internal/config"
	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/nlpsvc"
	"github.com/orion-cx/orion-backend/internal/platform/bus"
	"github.com/orion-cx/orion-backend/internal/platform/cache"
	"github.com/orion-cx/orion-backend/internal/platform/httpx"
	"github.com/orion-cx/orion-backend/internal/platform/security"
	"github.com/orion-cx/orion-backend/internal/platform/wsx"
)

// consumerGroup identifies the gateway on the event bus. It subscribes so that
// an event published by any service — including one running in another
// container — reaches the connected dashboards in real time.
const consumerGroup = "orion-gateway"

// Service is the orchestrator.
type Service struct {
	cfg      config.Config
	auth     authClient
	nlp      nlpClient
	calls    callClient
	notifies notifyClient
	sessions cache.SessionStore
	bus      bus.Bus
	hub      *wsx.Hub
	tokens   *security.Tokens
	logger   *slog.Logger

	// alertMu guards the dashboard alert banner, which is presentation state
	// shared by every agent rather than per-conversation data.
	alertMu      sync.RWMutex
	alertVisible bool
}

// NewService wires the gateway to its peers.
func NewService(cfg config.Config, tokens *security.Tokens, sessions cache.SessionStore, eventBus bus.Bus, logger *slog.Logger) *Service {
	newClient := func(baseURL string) *httpx.Client {
		return httpx.NewClient(baseURL, cfg.InternalTimeout, cfg.InternalRetries, logger)
	}
	service := &Service{
		cfg:          cfg,
		auth:         authClient{client: newClient(cfg.AuthURL)},
		nlp:          nlpClient{client: newClient(cfg.NLPURL), local: nlpsvc.NewRuleClassifier(), logger: logger},
		calls:        callClient{client: newClient(cfg.CallMgmtURL)},
		notifies:     notifyClient{client: newClient(cfg.NotificationURL)},
		sessions:     sessions,
		bus:          eventBus,
		hub:          wsx.NewHub(logger),
		tokens:       tokens,
		logger:       logger,
		alertVisible: true,
	}
	service.hub.OnFirstFrame = func(client *wsx.Client) any {
		snapshot, err := service.Snapshot(context.Background(), client.UserID, domain.Role(client.Role))
		if err != nil {
			logger.Warn("failed to build initial snapshot", slog.String("err", err.Error()))
			return nil
		}
		return snapshot
	}
	return service
}

// Start subscribes the gateway to the bus so remote events reach the sockets.
func (s *Service) Start(ctx context.Context) error {
	return s.bus.Subscribe(ctx, consumerGroup, func(eventCtx context.Context, event bus.Event) {
		s.hub.Each(func(client *wsx.Client) {
			// A customer only sees events about their own conversations.
			if domain.Role(client.Role) != domain.RoleAgent && event.UserID != client.UserID {
				return
			}
			client.Send(map[string]any{"event": "domainEvent", "payload": event})
		})
		s.broadcastSnapshots(eventCtx)
	})
}

// Snapshot is the full state one client is allowed to see. The shape is the
// contract the Flutter client decodes.
type Snapshot struct {
	Event            string                `json:"event"`
	Cases            []domain.Conversation `json:"cases"`
	Tickets          []domain.Ticket       `json:"tickets"`
	Notifications    []domain.Notification `json:"notifications"`
	LiveAlertVisible bool                  `json:"liveAlertVisible"`
	Threshold        float64               `json:"confidenceThreshold"`
}

// Snapshot builds the state for one principal.
func (s *Service) Snapshot(ctx context.Context, userID string, role domain.Role) (Snapshot, error) {
	snapshot := Snapshot{
		Event:            "snapshot",
		Cases:            []domain.Conversation{},
		Tickets:          []domain.Ticket{},
		Notifications:    []domain.Notification{},
		LiveAlertVisible: s.alertIsVisible(),
		Threshold:        s.cfg.ConfidenceThreshold,
	}

	// An agent sees the whole queue; a customer sees only their own history.
	ownerFilter := userID
	if role == domain.RoleAgent {
		ownerFilter = ""
	}

	conversations, err := s.calls.List(ctx, ownerFilter)
	if err != nil {
		return snapshot, err
	}
	snapshot.Cases = conversations

	if tickets, err := s.calls.Tickets(ctx, ownerFilter); err == nil {
		snapshot.Tickets = tickets
	} else {
		s.logger.Warn("snapshot without tickets", slog.String("err", err.Error()))
	}
	if userID != "" {
		if notifications, err := s.notifies.List(ctx, userID); err == nil {
			snapshot.Notifications = notifications
		} else {
			s.logger.Warn("snapshot without notifications", slog.String("err", err.Error()))
		}
	}
	return snapshot, nil
}

// broadcastSnapshots pushes a fresh, role-appropriate snapshot to every socket.
func (s *Service) broadcastSnapshots(ctx context.Context) {
	s.hub.Each(func(client *wsx.Client) {
		snapshot, err := s.Snapshot(ctx, client.UserID, domain.Role(client.Role))
		if err != nil {
			s.logger.Warn("failed to build snapshot for client", slog.String("err", err.Error()))
			return
		}
		client.Send(snapshot)
	})
}

// ActiveConversation returns the customer's open conversation, creating one on
// first contact. The lookup goes through the session context, which is what
// lets any channel land on the same conversation (RF003).
func (s *Service) ActiveConversation(ctx context.Context, principal security.Principal, channel domain.Channel) (domain.Conversation, error) {
	session, found, err := s.sessions.Get(ctx, principal.UserID)
	if err != nil {
		s.logger.Warn("session store unavailable, falling back to Call Management",
			slog.String("err", err.Error()))
	}
	if found && session.ConversationID != "" {
		conversation, err := s.calls.Get(ctx, session.ConversationID)
		if err == nil && conversation.Status != domain.StatusResolved {
			return conversation, nil
		}
	}

	// No usable context: reuse the newest unresolved conversation if there is
	// one, so a lost Redis entry never fragments a customer's history.
	conversations, err := s.calls.List(ctx, principal.UserID,
		domain.StatusBot, domain.StatusWaitingHuman, domain.StatusInProgress)
	if err != nil {
		return domain.Conversation{}, err
	}
	if len(conversations) > 0 {
		conversation := conversations[len(conversations)-1]
		s.saveSession(ctx, principal.UserID, conversation, channel, "")
		return conversation, nil
	}

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
	s.saveSession(ctx, principal.UserID, conversation, channel, "")

	// Greet on the very first contact so the channel is never an empty screen.
	greeted, err := s.calls.ApplyTurn(ctx, conversation.ID, callsvc.Turn{
		Messages: []callsvc.MessageInput{{
			Actor:   domain.ActorAssistant,
			Text:    "Olá! Eu sou o assistente Orion. Como posso ajudar com sua internet ou sua fatura hoje?",
			Channel: channel,
		}},
	})
	if err != nil {
		return conversation, nil
	}
	return greeted, nil
}

// HandleCustomerMessage is the heart of the platform: it classifies, routes,
// persists and announces one customer turn (RF004, RF005, RF008).
func (s *Service) HandleCustomerMessage(ctx context.Context, principal security.Principal, conversationID, text string, channel domain.Channel) (domain.Conversation, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return domain.Conversation{}, httpx.BadRequest("Escreva uma mensagem para continuar.")
	}
	if !channel.Valid() {
		return domain.Conversation{}, httpx.BadRequest("Canal inválido.")
	}

	conversation, err := s.calls.Get(ctx, conversationID)
	if err != nil {
		return domain.Conversation{}, err
	}
	if conversation.UserID != principal.UserID && principal.Role != domain.RoleAgent {
		return domain.Conversation{}, httpx.ErrForbidden
	}

	// A human owns the conversation: record the message and raise the badge,
	// but do not let the bot answer over the agent.
	if conversation.Status == domain.StatusWaitingHuman || conversation.Status == domain.StatusInProgress {
		unread := true
		updated, err := s.calls.ApplyTurn(ctx, conversationID, callsvc.Turn{
			Messages:       []callsvc.MessageInput{{Actor: domain.ActorCustomer, Text: text, Channel: channel}},
			HasUnreadEvent: &unread,
		})
		if err != nil {
			return domain.Conversation{}, err
		}
		s.setAlertVisible(true)
		s.broadcastSnapshots(ctx)
		return updated, nil
	}

	pendingAction := ""
	if conversation.PendingAction != nil {
		pendingAction = *conversation.PendingAction
	}
	session, _, _ := s.sessions.Get(ctx, principal.UserID)

	classification := s.nlp.Classify(ctx, text,
		pendingAction == domain.ActionRestartSignal || pendingAction == domain.ActionContinue)
	decision := decide(classification, s.cfg.ConfidenceThreshold, pendingAction, session.UnclearTurns)

	turn := callsvc.Turn{
		Messages: []callsvc.MessageInput{
			{Actor: domain.ActorCustomer, Text: text, Channel: channel},
			{Actor: domain.ActorAssistant, Text: decision.reply, Channel: channel},
		},
		Intent:           &classification.Intent,
		IntentConfidence: &classification.Confidence,
		Summary:          &classification.Summary,
		SetPendingAction: true,
		PendingAction:    decision.pendingAction,
	}

	switch {
	case decision.handoff:
		status := domain.StatusWaitingHuman
		unread := true
		turn.Status = &status
		turn.HasUnreadEvent = &unread
		turn.PendingAction = domain.ActionHumanHandoff
		turn.Messages = append(turn.Messages, callsvc.MessageInput{
			Actor:   domain.ActorSystem,
			Text:    "Evento REQUIRED_HUMAN_ASSISTANCE publicado. " + decision.reason,
			Channel: channel,
		})
	case decision.resolves:
		status := domain.StatusResolved
		turn.Status = &status
	}

	updated, err := s.calls.ApplyTurn(ctx, conversationID, turn)
	if err != nil {
		return domain.Conversation{}, err
	}

	// The protocol is opened after the turn is committed, so a ticket can never
	// reference a conversation state that was rolled back.
	if decision.ticketTitle != "" {
		if _, err := s.calls.OpenTicket(ctx, callsvc.TicketInput{
			UserID:         updated.UserID,
			ConversationID: updated.ID,
			Title:          decision.ticketTitle,
			Category:       decision.ticketKind,
			Status:         ticketStatusFor(updated.Status),
			Channel:        channel,
			FirstEvent:     "Chamado aberto pelo canal " + channel.Label() + ".",
		}); err != nil {
			s.logger.Warn("failed to open ticket", slog.String("err", err.Error()))
		}
	}

	if decision.handoff {
		event := bus.NewEvent(bus.EventHumanAssistanceRequired)
		event.UserID = updated.UserID
		event.ConversationID = updated.ID
		event.Channel = string(channel)
		event.Payload = map[string]any{
			"intent":     classification.Intent,
			"confidence": classification.Confidence,
			"threshold":  s.cfg.ConfidenceThreshold,
			"engine":     classification.Engine,
			"reason":     decision.reason,
			"summary":    classification.Summary,
		}
		if err := s.bus.Publish(ctx, event); err != nil {
			s.logger.Error("failed to publish handoff event", slog.String("err", err.Error()))
		}
		s.setAlertVisible(true)
	}

	// Track consecutive unclear turns so the second one escalates.
	unclear := session.UnclearTurns + 1
	if decision.handoff || decision.pendingAction != "" || decision.resolves ||
		(classification.Intent != nlpsvc.IntentUnknown && classification.Confidence >= s.cfg.ConfidenceThreshold) {
		unclear = 0
	}
	s.saveSessionWithUnclear(ctx, principal.UserID, updated, channel, classification.Intent, unclear)
	s.broadcastSnapshots(ctx)
	return updated, nil
}

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

// SwitchChannel resumes a journey on a different channel (RF003, flow A).
func (s *Service) SwitchChannel(ctx context.Context, principal security.Principal, conversationID string, from, to domain.Channel) (domain.Conversation, error) {
	if !to.Valid() {
		return domain.Conversation{}, httpx.BadRequest("Canal inválido.")
	}
	conversation, err := s.calls.Get(ctx, conversationID)
	if err != nil {
		return domain.Conversation{}, err
	}
	if conversation.UserID != principal.UserID && principal.Role != domain.RoleAgent {
		return domain.Conversation{}, httpx.ErrForbidden
	}

	session, _, _ := s.sessions.Get(ctx, principal.UserID)
	if from == "" || !from.Valid() {
		from = session.LastChannel
	}
	if from == "" {
		from = conversation.LastChannel()
	}

	messages := []callsvc.MessageInput{{
		Actor:   domain.ActorSystem,
		Text:    "Sessão " + conversation.ID + " recuperada de " + from.Label() + " em " + to.Label() + ".",
		Channel: to,
	}}

	turn := callsvc.Turn{Messages: messages}
	pendingAction := ""
	if conversation.PendingAction != nil {
		pendingAction = *conversation.PendingAction
	}

	switch pendingAction {
	case domain.ActionRestartSignal, domain.ActionContinue:
		turn.SetPendingAction = true
		turn.PendingAction = domain.ActionContinue
		turn.Messages = append(turn.Messages, callsvc.MessageInput{
			Actor:   domain.ActorAssistant,
			Text:    "Encontrei uma ação pendente: reiniciar o sinal. Quer continuar por aqui?",
			Channel: to,
		})
	case domain.ActionHumanHandoff:
		turn.Messages = append(turn.Messages, callsvc.MessageInput{
			Actor:   domain.ActorAssistant,
			Text:    "Seu atendimento está na fila com um especialista. Assim que houver resposta, ela chega por aqui.",
			Channel: to,
		})
	default:
		turn.Messages = append(turn.Messages, callsvc.MessageInput{
			Actor:   domain.ActorAssistant,
			Text:    "Seu contexto foi mantido. Podemos continuar o atendimento deste ponto.",
			Channel: to,
		})
	}

	updated, err := s.calls.ApplyTurn(ctx, conversationID, turn)
	if err != nil {
		return domain.Conversation{}, err
	}
	s.saveSession(ctx, principal.UserID, updated, to, session.LastIntent)
	s.broadcastSnapshots(ctx)
	return updated, nil
}

// ConfirmPendingAction executes the action the customer had left open. It is
// the button equivalent of answering "sim".
func (s *Service) ConfirmPendingAction(ctx context.Context, principal security.Principal, conversationID string, channel domain.Channel) (domain.Conversation, error) {
	return s.answerPendingAction(ctx, principal, conversationID, channel, true)
}

// DeclinePendingAction cancels the pending action.
func (s *Service) DeclinePendingAction(ctx context.Context, principal security.Principal, conversationID string, channel domain.Channel) (domain.Conversation, error) {
	return s.answerPendingAction(ctx, principal, conversationID, channel, false)
}

func (s *Service) answerPendingAction(ctx context.Context, principal security.Principal, conversationID string, channel domain.Channel, accept bool) (domain.Conversation, error) {
	conversation, err := s.calls.Get(ctx, conversationID)
	if err != nil {
		return domain.Conversation{}, err
	}
	if conversation.UserID != principal.UserID && principal.Role != domain.RoleAgent {
		return domain.Conversation{}, httpx.ErrForbidden
	}
	pendingAction := ""
	if conversation.PendingAction != nil {
		pendingAction = *conversation.PendingAction
	}
	if pendingAction != domain.ActionRestartSignal && pendingAction != domain.ActionContinue {
		return conversation, nil
	}
	if !channel.Valid() {
		channel = conversation.LastChannel()
	}

	turn := callsvc.Turn{SetPendingAction: true}
	if accept {
		summary := "Cliente relatou lentidão, autorizou o reinício do sinal e recebeu a confirmação da execução."
		status := domain.StatusResolved
		turn.Messages = []callsvc.MessageInput{
			{Actor: domain.ActorCustomer, Text: "Sim, pode reiniciar.", Channel: channel},
			{Actor: domain.ActorAssistant, Text: "Pronto! O sinal foi reiniciado. Aguarde cerca de 30 segundos e teste a conexão novamente.", Channel: channel},
		}
		turn.Summary = &summary
		turn.Status = &status
	} else {
		turn.Messages = []callsvc.MessageInput{
			{Actor: domain.ActorCustomer, Text: "Agora não.", Channel: channel},
			{Actor: domain.ActorAssistant, Text: "Sem problema. A ação foi cancelada e o atendimento continua disponível.", Channel: channel},
		}
	}

	updated, err := s.calls.ApplyTurn(ctx, conversationID, turn)
	if err != nil {
		return domain.Conversation{}, err
	}
	s.saveSession(ctx, principal.UserID, updated, channel, updated.Intent)
	s.broadcastSnapshots(ctx)
	return updated, nil
}

// saveSession stores the low-latency context used to resume a journey.
func (s *Service) saveSession(ctx context.Context, userID string, conversation domain.Conversation, channel domain.Channel, intent string) {
	s.saveSessionWithUnclear(ctx, userID, conversation, channel, intent, 0)
}

func (s *Service) saveSessionWithUnclear(ctx context.Context, userID string, conversation domain.Conversation, channel domain.Channel, intent string, unclearTurns int) {
	pendingAction := ""
	if conversation.PendingAction != nil {
		pendingAction = *conversation.PendingAction
	}
	if err := s.sessions.Save(ctx, domain.SessionContext{
		SessionID:      "SES-" + conversation.ID,
		UserID:         userID,
		ConversationID: conversation.ID,
		LastChannel:    channel,
		PendingAction:  pendingAction,
		LastIntent:     intent,
		UnclearTurns:   unclearTurns,
	}); err != nil {
		// The journey still works without the cache: ActiveConversation falls
		// back to Call Management. Losing the context only costs a lookup.
		s.logger.Warn("failed to persist session context", slog.String("err", err.Error()))
	}
}

func (s *Service) alertIsVisible() bool {
	s.alertMu.RLock()
	defer s.alertMu.RUnlock()
	return s.alertVisible
}

func (s *Service) setAlertVisible(visible bool) {
	s.alertMu.Lock()
	s.alertVisible = visible
	s.alertMu.Unlock()
}
