// Package e2e wires the five Orion services together in one process and drives
// them through the two acceptance flows of the challenge over real HTTP.
//
// Every service here is the production code path: real handlers, real routing,
// real event bus, real session store. Only the storage backend differs
// (in-memory instead of PostgreSQL), so the test needs no Docker.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/orion-cx/orion-backend/internal/authsvc"
	"github.com/orion-cx/orion-backend/internal/callsvc"
	"github.com/orion-cx/orion-backend/internal/config"
	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/gatewaysvc"
	"github.com/orion-cx/orion-backend/internal/nlpsvc"
	"github.com/orion-cx/orion-backend/internal/notifysvc"
	"github.com/orion-cx/orion-backend/internal/platform/bus"
	"github.com/orion-cx/orion-backend/internal/platform/cache"
	"github.com/orion-cx/orion-backend/internal/platform/security"
)

// platform is the whole stack, reachable through the gateway URL.
type platform struct {
	authURL    string
	gatewayURL string
	client     *http.Client
	t          *testing.T
}

func newPlatform(t *testing.T) *platform {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	eventBus := bus.NewInProcess(logger)
	tokens := security.NewTokens("test-secret", time.Hour)
	ctx := context.Background()

	// bcrypt cost 4 keeps the suite fast; production uses the configured cost.
	auth := authsvc.NewService(authsvc.NewMemoryRepository(), tokens, 4, logger)
	authServer := httptest.NewServer(auth.Routes())
	t.Cleanup(authServer.Close)

	calls := callsvc.NewService(callsvc.NewMemoryRepository(), eventBus, logger)
	callServer := httptest.NewServer(calls.Routes())
	t.Cleanup(callServer.Close)

	// No API key: the NLP service runs its rule engine, which is the path the
	// prototype takes by default.
	nlp := nlpsvc.NewService(nil, logger)
	nlpServer := httptest.NewServer(nlp.Routes())
	t.Cleanup(nlpServer.Close)

	notifications := notifysvc.NewService(notifysvc.NewMemoryRepository(), eventBus, logger)
	if err := notifications.Start(ctx); err != nil {
		t.Fatalf("start notification consumer: %v", err)
	}
	notifyServer := httptest.NewServer(notifications.Routes())
	t.Cleanup(notifyServer.Close)

	cfg := config.Config{
		Env:                 "test",
		AuthURL:             authServer.URL,
		CallMgmtURL:         callServer.URL,
		NLPURL:              nlpServer.URL,
		NotificationURL:     notifyServer.URL,
		ConfidenceThreshold: 0.70,
		InternalTimeout:     5 * time.Second,
		InternalRetries:     1,
		AllowedOrigin:       []string{"*"},
	}

	gateway := gatewaysvc.NewService(cfg, tokens, cache.NewMemory(), eventBus, logger)
	if err := gateway.Start(ctx); err != nil {
		t.Fatalf("start gateway consumer: %v", err)
	}
	gatewayServer := httptest.NewServer(gateway.Routes())
	t.Cleanup(gatewayServer.Close)

	return &platform{gatewayURL: gatewayServer.URL, authURL: authServer.URL, client: gatewayServer.Client(), t: t}
}

// do issues an authenticated request against the gateway and decodes the body.
func (p *platform) do(method, path, token string, body, out any) int {
	p.t.Helper()

	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			p.t.Fatalf("encode body: %v", err)
		}
		payload = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, p.gatewayURL+path, payload)
	if err != nil {
		p.t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	response, err := p.client.Do(request)
	if err != nil {
		p.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = response.Body.Close() }()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		p.t.Fatalf("read body: %v", err)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			p.t.Fatalf("decode %s %s (%d): %v — body: %s", method, path, response.StatusCode, err, raw)
		}
	}
	return response.StatusCode
}

// register creates an account and returns its session token.
//
// Customers sign up through the public gateway endpoint, which is what a real
// channel does. Agent accounts cannot be created that way — the gateway forces
// every public registration to the customer role — so they are provisioned
// against the Authenticator on the internal network, exactly like the seeder
// does in the running stack.
func (p *platform) register(email, name string, role domain.Role) (authsvc.Session, string) {
	p.t.Helper()

	input := authsvc.RegisterInput{
		Email:        email,
		Password:     "orion12345",
		Name:         name,
		DocumentMask: "***.482.***-**",
		PlanName:     "Claro Fibra 500 Mega",
		Role:         role,
	}

	target := p.gatewayURL + "/api/auth/register"
	if role == domain.RoleAgent {
		target = p.authURL + "/v1/register"
	}

	encoded, err := json.Marshal(input)
	if err != nil {
		p.t.Fatalf("encode registration: %v", err)
	}
	response, err := p.client.Post(target, "application/json", bytes.NewReader(encoded))
	if err != nil {
		p.t.Fatalf("register %s: %v", email, err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		p.t.Fatalf("register %s: unexpected status %d — %s", email, response.StatusCode, body)
	}
	var session authsvc.Session
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		p.t.Fatalf("decode registration: %v", err)
	}
	return session, session.Token
}

// TestPublicRegistrationCannotCreateAgents pins the privilege-escalation guard:
// asking for the agent role on the public endpoint must yield a customer.
func TestPublicRegistrationCannotCreateAgents(t *testing.T) {
	platform := newPlatform(t)

	var elevated authsvc.Session
	status := platform.do("POST", "/api/auth/register", "", authsvc.RegisterInput{
		Email:    "escalada@orion.test",
		Password: "orion12345",
		Name:     "Tentativa Pública",
		Role:     domain.RoleAgent,
	}, &elevated)
	if status != http.StatusCreated {
		t.Fatalf("expected the registration to succeed, got %d", status)
	}
	if elevated.User.Role != domain.RoleCustomer {
		t.Fatalf("public registration must not grant the agent role, got %s", elevated.User.Role)
	}
}

// snapshot is the shape the gateway returns for /api/state.
type snapshot struct {
	Cases         []domain.Conversation `json:"cases"`
	Tickets       []domain.Ticket       `json:"tickets"`
	Notifications []domain.Notification `json:"notifications"`
}

func (p *platform) state(token, channel string) snapshot {
	p.t.Helper()
	var result snapshot
	if status := p.do("GET", "/api/state?channel="+channel, token, nil, &result); status != http.StatusOK {
		p.t.Fatalf("state: unexpected status %d", status)
	}
	return result
}

func lastMessageOf(conversation domain.Conversation, actor domain.Actor) string {
	for index := len(conversation.Messages) - 1; index >= 0; index-- {
		if conversation.Messages[index].Actor == actor {
			return conversation.Messages[index].Text
		}
	}
	return ""
}

// TestFlowA_TechnicalSupportAcrossChannels covers acceptance flow A: a customer
// reports slow internet on WhatsApp, leaves the question unanswered, comes back
// on the Web Portal and finishes the journey there with the context intact.
func TestFlowA_TechnicalSupportAcrossChannels(t *testing.T) {
	platform := newPlatform(t)
	_, token := platform.register("flowa@orion.test", "Cliente Fluxo A", domain.RoleCustomer)

	// Step 1-2: first contact on WhatsApp opens a session.
	state := platform.state(token, string(domain.ChannelWhatsApp))
	if len(state.Cases) != 1 {
		t.Fatalf("expected one conversation after first contact, got %d", len(state.Cases))
	}
	conversationID := state.Cases[0].ID

	// Step 3-4: the message is classified and the assistant asks the triage
	// question, storing the pending action.
	var afterMessage domain.Conversation
	platform.do("POST", "/api/cases/"+conversationID+"/messages", token,
		map[string]string{"text": "minha internet está lenta", "channel": string(domain.ChannelWhatsApp)},
		&afterMessage)

	if afterMessage.Intent != nlpsvc.IntentTechnicalSupport {
		t.Fatalf("expected intent %s, got %s", nlpsvc.IntentTechnicalSupport, afterMessage.Intent)
	}
	if afterMessage.IntentConfidence < 0.70 {
		t.Fatalf("expected high confidence, got %v", afterMessage.IntentConfidence)
	}
	if afterMessage.Status != domain.StatusBot {
		t.Fatalf("a confident technical request must stay automated, got %s", afterMessage.Status)
	}
	if afterMessage.PendingAction == nil || *afterMessage.PendingAction != domain.ActionRestartSignal {
		t.Fatalf("expected pending action %s, got %v", domain.ActionRestartSignal, afterMessage.PendingAction)
	}
	if !strings.Contains(lastMessageOf(afterMessage, domain.ActorAssistant), "reiniciar o sinal") {
		t.Fatalf("expected the triage question, got %q", lastMessageOf(afterMessage, domain.ActorAssistant))
	}

	// Step 6: the customer shows up on another channel. The platform must
	// recognise the journey and offer to resume it.
	var afterSwitch domain.Conversation
	platform.do("POST", "/api/cases/"+conversationID+"/switch-channel", token,
		map[string]string{"channel": string(domain.ChannelWeb), "previousChannel": string(domain.ChannelWhatsApp)},
		&afterSwitch)

	if afterSwitch.PendingAction == nil || *afterSwitch.PendingAction != domain.ActionContinue {
		t.Fatalf("expected the pending action to become resumable, got %v", afterSwitch.PendingAction)
	}
	if !strings.Contains(lastMessageOf(afterSwitch, domain.ActorSystem), "recuperada de WhatsApp em Web Portal") {
		t.Fatalf("expected the channel handover to be recorded in the history")
	}
	if !strings.Contains(lastMessageOf(afterSwitch, domain.ActorAssistant), "continuar por aqui") {
		t.Fatalf("expected the assistant to offer resuming the journey")
	}

	// Step 7: the customer confirms on the new channel and the action runs.
	var afterConfirm domain.Conversation
	platform.do("POST", "/api/cases/"+conversationID+"/continue-here", token,
		map[string]string{"channel": string(domain.ChannelWeb)}, &afterConfirm)

	if afterConfirm.Status != domain.StatusResolved {
		t.Fatalf("expected the conversation to be resolved, got %s", afterConfirm.Status)
	}
	if afterConfirm.PendingAction != nil {
		t.Fatalf("expected no pending action after completion, got %v", *afterConfirm.PendingAction)
	}

	// The history is one continuous thread spanning both channels (RF002).
	channels := map[domain.Channel]bool{}
	for _, message := range afterConfirm.Messages {
		channels[message.Channel] = true
	}
	if !channels[domain.ChannelWhatsApp] || !channels[domain.ChannelWeb] {
		t.Fatalf("expected one history covering both channels, got %v", channels)
	}

	// The protocol follows the conversation to a closed state (RF006).
	var tickets []domain.Ticket
	platform.do("GET", "/api/tickets", token, nil, &tickets)
	if len(tickets) != 1 {
		t.Fatalf("expected one ticket, got %d", len(tickets))
	}
	if tickets[0].Status != domain.TicketResolved {
		t.Fatalf("expected the ticket to be resolved with the conversation, got %s", tickets[0].Status)
	}
}

// TestFlowB_LowConfidenceHumanHandoff covers acceptance flow B: a billing
// dispute is classified below the automation threshold, an event is published,
// an agent takes the case from the dashboard and answers manually.
func TestFlowB_LowConfidenceHumanHandoff(t *testing.T) {
	platform := newPlatform(t)
	_, customerToken := platform.register("flowb@orion.test", "Cliente Fluxo B", domain.RoleCustomer)
	agentSession, agentToken := platform.register("agente@orion.test", "Camila Rocha", domain.RoleAgent)

	if agentSession.User.Role != domain.RoleAgent {
		t.Fatalf("expected an agent account, got role %s", agentSession.User.Role)
	}

	state := platform.state(customerToken, string(domain.ChannelApp))
	conversationID := state.Cases[0].ID

	// Steps 2-3: low confidence classification triggers the handoff.
	var afterMessage domain.Conversation
	platform.do("POST", "/api/cases/"+conversationID+"/messages", customerToken,
		map[string]string{"text": "quero contestar uma cobrança indevida", "channel": string(domain.ChannelApp)},
		&afterMessage)

	if afterMessage.Intent != nlpsvc.IntentBillingDispute {
		t.Fatalf("expected intent %s, got %s", nlpsvc.IntentBillingDispute, afterMessage.Intent)
	}
	if afterMessage.IntentConfidence >= 0.70 {
		t.Fatalf("expected confidence below the automation threshold, got %v", afterMessage.IntentConfidence)
	}
	if afterMessage.Status != domain.StatusWaitingHuman {
		t.Fatalf("expected the conversation to enter the human queue, got %s", afterMessage.Status)
	}
	if !afterMessage.HasUnreadEvent {
		t.Fatal("expected the queue entry to be flagged as unread for the dashboard")
	}
	if !strings.Contains(lastMessageOf(afterMessage, domain.ActorSystem), "REQUIRED_HUMAN_ASSISTANCE") {
		t.Fatalf("expected the handoff event to be recorded in the history")
	}

	// Step 4: the agent dashboard sees the queue entry.
	agentState := platform.state(agentToken, "")
	found := false
	for _, conversation := range agentState.Cases {
		if conversation.ID == conversationID && conversation.Status == domain.StatusWaitingHuman {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the waiting conversation on the agent dashboard")
	}

	// A customer must not be able to reach the dashboard actions.
	if status := platform.do("POST", "/api/cases/"+conversationID+"/take", customerToken,
		map[string]string{}, nil); status != http.StatusForbidden {
		t.Fatalf("expected 403 for a customer taking a case, got %d", status)
	}

	// Step 5: the agent takes the case and answers manually.
	var afterTake domain.Conversation
	platform.do("POST", "/api/cases/"+conversationID+"/take", agentToken, map[string]string{}, &afterTake)
	if afterTake.Status != domain.StatusInProgress {
		t.Fatalf("expected the case to be in progress, got %s", afterTake.Status)
	}
	if afterTake.AssignedAgent == nil || *afterTake.AssignedAgent != "Camila Rocha" {
		t.Fatalf("expected the case to be assigned to the acting agent, got %v", afterTake.AssignedAgent)
	}

	const reply = "Localizei a cobrança e solicitei o estorno na próxima fatura."
	var afterReply domain.Conversation
	platform.do("POST", "/api/cases/"+conversationID+"/agent-messages", agentToken,
		map[string]string{"text": reply}, &afterReply)

	// Step 6: the answer goes back on the channel the customer was using.
	last := afterReply.Messages[len(afterReply.Messages)-1]
	if last.Actor != domain.ActorAgent || last.Text != reply {
		t.Fatalf("expected the agent reply at the end of the history, got %+v", last)
	}
	if last.Channel != domain.ChannelApp {
		t.Fatalf("expected the reply on the customer channel %s, got %s", domain.ChannelApp, last.Channel)
	}

	// The customer sees the same thread from their own session.
	customerState := platform.state(customerToken, string(domain.ChannelApp))
	var customerView domain.Conversation
	for _, conversation := range customerState.Cases {
		if conversation.ID == conversationID {
			customerView = conversation
		}
	}
	if lastMessageOf(customerView, domain.ActorAgent) != reply {
		t.Fatalf("expected the customer to see the agent reply")
	}

	// Step 7: closing the case ends the conversation.
	var afterResolve domain.Conversation
	platform.do("POST", "/api/cases/"+conversationID+"/resolve", agentToken, nil, &afterResolve)
	if afterResolve.Status != domain.StatusResolved {
		t.Fatalf("expected the conversation to be resolved, got %s", afterResolve.Status)
	}

	// The bus is asynchronous, so the notifications produced by the handoff
	// need a moment to land.
	deadline := time.Now().Add(3 * time.Second)
	var notifications []domain.Notification
	for time.Now().Before(deadline) {
		platform.do("GET", "/api/notifications", customerToken, nil, &notifications)
		if len(notifications) >= 3 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	titles := make([]string, 0, len(notifications))
	for _, notification := range notifications {
		titles = append(titles, notification.Title)
	}
	joined := strings.Join(titles, " | ")
	for _, expected := range []string{"transferido", "assumiu", "Nova resposta"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected a notification mentioning %q, got: %s", expected, joined)
		}
	}
}

// TestUnclearMessageAsksBeforeEscalating checks the routing rule that a single
// ambiguous sentence produces a clarifying question, and only the second one
// escalates to a human.
func TestUnclearMessageAsksBeforeEscalating(t *testing.T) {
	platform := newPlatform(t)
	_, token := platform.register("vago@orion.test", "Cliente Vago", domain.RoleCustomer)
	conversationID := platform.state(token, string(domain.ChannelWeb)).Cases[0].ID

	var first domain.Conversation
	platform.do("POST", "/api/cases/"+conversationID+"/messages", token,
		map[string]string{"text": "preciso de uma ajuda aí", "channel": string(domain.ChannelWeb)}, &first)
	if first.Status != domain.StatusBot {
		t.Fatalf("expected the first unclear message to stay automated, got %s", first.Status)
	}

	var second domain.Conversation
	platform.do("POST", "/api/cases/"+conversationID+"/messages", token,
		map[string]string{"text": "é sobre aquilo que falei", "channel": string(domain.ChannelWeb)}, &second)
	if second.Status != domain.StatusWaitingHuman {
		t.Fatalf("expected the second unclear message to escalate, got %s", second.Status)
	}
}

// TestLGPDErasure checks that a customer can have their data removed across
// every service.
func TestLGPDErasure(t *testing.T) {
	platform := newPlatform(t)
	_, token := platform.register("apagar@orion.test", "Cliente Apagar", domain.RoleCustomer)
	conversationID := platform.state(token, string(domain.ChannelApp)).Cases[0].ID
	platform.do("POST", "/api/cases/"+conversationID+"/messages", token,
		map[string]string{"text": "minha internet está lenta", "channel": string(domain.ChannelApp)}, nil)

	if status := platform.do("DELETE", "/api/auth/me", token, nil, nil); status != http.StatusOK {
		t.Fatalf("expected the erasure request to succeed, got %d", status)
	}

	// The conversation is gone and the account can no longer authenticate.
	if status := platform.do("GET", "/api/cases/"+conversationID, token, nil, nil); status != http.StatusNotFound {
		t.Fatalf("expected the conversation to be removed, got %d", status)
	}
	if status := platform.do("POST", "/api/auth/login", "",
		map[string]string{"email": "apagar@orion.test", "password": "orion12345"}, nil); status != http.StatusUnauthorized {
		t.Fatalf("expected the anonymized account to be unable to log in, got %d", status)
	}
}
