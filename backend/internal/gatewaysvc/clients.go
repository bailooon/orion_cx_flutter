// Package gatewaysvc is the ORION Gateway: the single public entry point and
// the orchestrator that coordinates the other four services.
package gatewaysvc

import (
	"context"
	"log/slog"
	"net/url"
	"time"

	"github.com/orion-cx/orion-backend/internal/authsvc"
	"github.com/orion-cx/orion-backend/internal/callsvc"
	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/nlpsvc"
	"github.com/orion-cx/orion-backend/internal/platform/httpx"
)

// authClient talks to ORION Authenticator.
type authClient struct{ client *httpx.Client }

func (c authClient) Login(ctx context.Context, email, password string) (authsvc.Session, error) {
	var session authsvc.Session
	err := c.client.Do(ctx, "POST", "/v1/login",
		map[string]string{"email": email, "password": password}, &session)
	return session, err
}

func (c authClient) Register(ctx context.Context, input authsvc.RegisterInput) (authsvc.Session, error) {
	var session authsvc.Session
	err := c.client.Do(ctx, "POST", "/v1/register", input, &session)
	return session, err
}

func (c authClient) Profile(ctx context.Context, userID string) (domain.User, error) {
	var user domain.User
	err := c.client.Do(ctx, "GET", "/v1/users/"+url.PathEscape(userID), nil, &user)
	return user, err
}

func (c authClient) Anonymize(ctx context.Context, userID string) error {
	return c.client.Do(ctx, "DELETE", "/v1/users/"+url.PathEscape(userID), nil, nil)
}

// nlpClient talks to ORION Motor NLP/IA.
type nlpClient struct {
	client *httpx.Client
	// local is the same rule engine the NLP service uses. If the whole NLP
	// service is unreachable the gateway still classifies locally instead of
	// leaving the customer without an answer (RNF007).
	local  *nlpsvc.RuleClassifier
	logger *slog.Logger
}

func (c nlpClient) Classify(ctx context.Context, text string, hasPendingQuestion bool) nlpsvc.Result {
	var result nlpsvc.Result
	err := c.client.Do(ctx, "POST", "/v1/classify",
		nlpsvc.Request{Text: text, HasPendingQuestion: hasPendingQuestion}, &result)
	if err == nil && result.Intent != "" {
		return result
	}
	if err != nil {
		c.logger.Warn("nlp service unreachable, classifying locally",
			slog.String("err", err.Error()))
	}
	result = c.local.Classify(text, hasPendingQuestion)
	result.Engine = "rules-gateway-fallback"
	return result
}

// callClient talks to ORION Call Management.
type callClient struct{ client *httpx.Client }

func (c callClient) Open(ctx context.Context, input callsvc.NewConversationInput) (domain.Conversation, error) {
	var conversation domain.Conversation
	err := c.client.Do(ctx, "POST", "/v1/conversations", input, &conversation)
	return conversation, err
}

func (c callClient) Get(ctx context.Context, id string) (domain.Conversation, error) {
	var conversation domain.Conversation
	err := c.client.Do(ctx, "GET", "/v1/conversations/"+url.PathEscape(id), nil, &conversation)
	return conversation, err
}

func (c callClient) List(ctx context.Context, userID string, statuses ...domain.ConversationStatus) ([]domain.Conversation, error) {
	query := url.Values{}
	if userID != "" {
		query.Set("userId", userID)
	}
	for _, status := range statuses {
		query.Add("status", string(status))
	}
	path := "/v1/conversations"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var conversations []domain.Conversation
	err := c.client.Do(ctx, "GET", path, nil, &conversations)
	return conversations, err
}

func (c callClient) ApplyTurn(ctx context.Context, id string, turn callsvc.Turn) (domain.Conversation, error) {
	var conversation domain.Conversation
	err := c.client.Do(ctx, "POST", "/v1/conversations/"+url.PathEscape(id)+"/turns", turn, &conversation)
	return conversation, err
}

func (c callClient) Assign(ctx context.Context, id, agentName string) (domain.Conversation, error) {
	var conversation domain.Conversation
	err := c.client.Do(ctx, "POST", "/v1/conversations/"+url.PathEscape(id)+"/assign",
		map[string]string{"agentName": agentName}, &conversation)
	return conversation, err
}

func (c callClient) AgentReply(ctx context.Context, id, agentName, text string) (domain.Conversation, error) {
	var conversation domain.Conversation
	err := c.client.Do(ctx, "POST", "/v1/conversations/"+url.PathEscape(id)+"/agent-messages",
		map[string]string{"agentName": agentName, "text": text}, &conversation)
	return conversation, err
}

func (c callClient) Resolve(ctx context.Context, id string) (domain.Conversation, error) {
	var conversation domain.Conversation
	err := c.client.Do(ctx, "POST", "/v1/conversations/"+url.PathEscape(id)+"/resolve", nil, &conversation)
	return conversation, err
}

func (c callClient) MarkRead(ctx context.Context, id string) (domain.Conversation, error) {
	var conversation domain.Conversation
	err := c.client.Do(ctx, "POST", "/v1/conversations/"+url.PathEscape(id)+"/mark-read", nil, &conversation)
	return conversation, err
}

func (c callClient) Reset(ctx context.Context, id string) (domain.Conversation, error) {
	var conversation domain.Conversation
	err := c.client.Do(ctx, "POST", "/v1/conversations/"+url.PathEscape(id)+"/reset", nil, &conversation)
	return conversation, err
}

func (c callClient) OpenTicket(ctx context.Context, input callsvc.TicketInput) (domain.Ticket, error) {
	var ticket domain.Ticket
	err := c.client.Do(ctx, "POST", "/v1/tickets", input, &ticket)
	return ticket, err
}

func (c callClient) Tickets(ctx context.Context, userID string) ([]domain.Ticket, error) {
	path := "/v1/tickets"
	if userID != "" {
		path += "?userId=" + url.QueryEscape(userID)
	}
	var tickets []domain.Ticket
	err := c.client.Do(ctx, "GET", path, nil, &tickets)
	return tickets, err
}

func (c callClient) PurgeUser(ctx context.Context, userID string) error {
	return c.client.Do(ctx, "DELETE", "/v1/users/"+url.PathEscape(userID)+"/data", nil, nil)
}

// notifyClient talks to ORION Notification.
type notifyClient struct{ client *httpx.Client }

func (c notifyClient) List(ctx context.Context, userID string) ([]domain.Notification, error) {
	var notifications []domain.Notification
	err := c.client.Do(ctx, "GET", "/v1/notifications?userId="+url.QueryEscape(userID), nil, &notifications)
	return notifications, err
}

func (c notifyClient) MarkRead(ctx context.Context, id string) error {
	return c.client.Do(ctx, "POST", "/v1/notifications/"+url.PathEscape(id)+"/read", nil, nil)
}

func (c notifyClient) MarkAllRead(ctx context.Context, userID string) error {
	return c.client.Do(ctx, "POST", "/v1/notifications/read-all?userId="+url.QueryEscape(userID), nil, nil)
}

func (c notifyClient) DeleteByUser(ctx context.Context, userID string) error {
	return c.client.Do(ctx, "DELETE", "/v1/users/"+url.PathEscape(userID)+"/notifications", nil, nil)
}

// healthCheck probes a peer service for the readiness endpoint.
func healthCheck(ctx context.Context, client *httpx.Client) string {
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var payload map[string]string
	if err := client.Do(probeCtx, "GET", "/health", nil, &payload); err != nil {
		return "unreachable"
	}
	if status, ok := payload["status"]; ok {
		return status
	}
	return "unknown"
}
