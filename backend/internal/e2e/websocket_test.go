package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/orion-cx/orion-backend/internal/domain"
)

// wsSnapshot is the frame the gateway pushes to a connected client.
type wsSnapshot struct {
	Event         string                `json:"event"`
	Cases         []domain.Conversation `json:"cases"`
	Notifications []domain.Notification `json:"notifications"`
}

// connect opens the dashboard socket. The token travels in the query string
// because a browser cannot set headers on a WebSocket handshake.
func (p *platform) connect(token string) *websocket.Conn {
	p.t.Helper()

	url := strings.Replace(p.gatewayURL, "http://", "ws://", 1) + "/ws?token=" + token
	conn, response, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		p.t.Fatalf("websocket dial failed (status %d): %v", status, err)
	}
	p.t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// readSnapshotUntil reads frames until one satisfies match, or the deadline
// passes. Frames that are not snapshots (domain events) are skipped.
func readSnapshotUntil(t *testing.T, conn *websocket.Conn, match func(wsSnapshot) bool) wsSnapshot {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := conn.SetReadDeadline(deadline); err != nil {
			t.Fatalf("set read deadline: %v", err)
		}
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("websocket read failed: %v", err)
		}
		var frame wsSnapshot
		if err := json.Unmarshal(raw, &frame); err != nil {
			continue
		}
		if frame.Event == "snapshot" && match(frame) {
			return frame
		}
	}
	t.Fatal("no matching snapshot arrived before the deadline")
	return wsSnapshot{}
}

// TestWebSocketRequiresAValidToken pins that the real-time channel is not an
// unauthenticated back door into the platform.
func TestWebSocketRequiresAValidToken(t *testing.T) {
	platform := newPlatform(t)

	url := strings.Replace(platform.gatewayURL, "http://", "ws://", 1)
	for name, target := range map[string]string{
		"sem token":      url + "/ws",
		"token inválido": url + "/ws?token=nao-e-um-token",
	} {
		t.Run(name, func(t *testing.T) {
			conn, response, err := websocket.DefaultDialer.Dial(target, nil)
			if err == nil {
				_ = conn.Close()
				t.Fatal("the handshake should have been rejected")
			}
			if response == nil || response.StatusCode != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %v", response)
			}
		})
	}
}

// TestDashboardReceivesHandoffInRealTime covers step 4 of acceptance flow B:
// the agent dashboard must learn about a handoff over the socket, without
// polling and without reloading.
func TestDashboardReceivesHandoffInRealTime(t *testing.T) {
	platform := newPlatform(t)
	_, customerToken := platform.register("ws-cliente@orion.test", "Cliente WS", domain.RoleCustomer)
	_, agentToken := platform.register("ws-agente@orion.test", "Camila Rocha", domain.RoleAgent)

	// The agent opens the dashboard before anything happens.
	agentSocket := platform.connect(agentToken)

	// The first frame is the initial snapshot, which lets the UI render
	// immediately instead of waiting for the next mutation.
	initial := readSnapshotUntil(t, agentSocket, func(wsSnapshot) bool { return true })
	for _, conversation := range initial.Cases {
		if conversation.Status == domain.StatusWaitingHuman {
			t.Fatal("the queue should start empty in this test")
		}
	}

	// The customer triggers the handoff from their own channel.
	conversationID := platform.state(customerToken, string(domain.ChannelApp)).Cases[0].ID
	platform.do("POST", "/api/cases/"+conversationID+"/messages", customerToken,
		map[string]string{"text": "quero contestar uma cobrança indevida", "channel": string(domain.ChannelApp)}, nil)

	// The dashboard is told about it without asking.
	pushed := readSnapshotUntil(t, agentSocket, func(frame wsSnapshot) bool {
		for _, conversation := range frame.Cases {
			if conversation.ID == conversationID &&
				conversation.Status == domain.StatusWaitingHuman {
				return true
			}
		}
		return false
	})

	var queued domain.Conversation
	for _, conversation := range pushed.Cases {
		if conversation.ID == conversationID {
			queued = conversation
		}
	}
	if !queued.HasUnreadEvent {
		t.Fatal("the pushed queue entry must be flagged as unread")
	}
	if queued.Summary == "" {
		t.Fatal("the agent must receive the AI summary with the queue entry")
	}
	if queued.IntentConfidence >= 0.70 {
		t.Fatalf("expected the handoff confidence below the threshold, got %v", queued.IntentConfidence)
	}
}

// TestCustomerSocketOnlySeesOwnConversations pins the scoping rule: a customer
// socket must never receive another customer's conversation.
func TestCustomerSocketOnlySeesOwnConversations(t *testing.T) {
	platform := newPlatform(t)
	firstSession, firstToken := platform.register("ws-a@orion.test", "Cliente A", domain.RoleCustomer)
	_, secondToken := platform.register("ws-b@orion.test", "Cliente B", domain.RoleCustomer)

	// Both customers start a conversation.
	platform.state(firstToken, string(domain.ChannelApp))
	secondCaseID := platform.state(secondToken, string(domain.ChannelWeb)).Cases[0].ID

	socket := platform.connect(firstToken)
	frame := readSnapshotUntil(t, socket, func(wsSnapshot) bool { return true })

	for _, conversation := range frame.Cases {
		if conversation.ID == secondCaseID {
			t.Fatal("a customer socket leaked another customer's conversation")
		}
		if conversation.UserID != firstSession.User.ID {
			t.Fatalf("unexpected conversation owner %s on the socket", conversation.UserID)
		}
	}
}
