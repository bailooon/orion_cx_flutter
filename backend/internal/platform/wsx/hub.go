// Package wsx is the WebSocket fan-out used by the agent dashboard and by the
// customer channels to receive state without polling (flow B, step 4).
package wsx

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 4096
	sendBuffer     = 32
)

// Client is one open socket.
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	UserID string
	Role   string
}

// Hub keeps the set of live sockets and broadcasts frames to them.
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
	logger  *slog.Logger

	// OnFirstFrame lets the owner send an initial snapshot to a client that
	// just connected, so the UI renders immediately instead of waiting for the
	// next mutation.
	OnFirstFrame func(client *Client) any
}

// NewHub builds an empty hub.
func NewHub(logger *slog.Logger) *Hub {
	return &Hub{clients: make(map[*Client]struct{}), logger: logger}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// The prototype is served from a different origin than the API during
	// development. Production pins this to the Claro domains via
	// ORION_ALLOWED_ORIGINS on the HTTP layer.
	CheckOrigin: func(*http.Request) bool { return true },
}

// Serve upgrades an HTTP request to a WebSocket and registers it.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, userID, role string) error {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}
	client := &Client{hub: h, conn: conn, send: make(chan []byte, sendBuffer), UserID: userID, Role: role}

	h.mu.Lock()
	h.clients[client] = struct{}{}
	total := len(h.clients)
	h.mu.Unlock()
	h.logger.Info("websocket connected", slog.String("role", role), slog.Int("clients", total))

	go client.writePump()
	go client.readPump()

	if h.OnFirstFrame != nil {
		if payload := h.OnFirstFrame(client); payload != nil {
			client.Send(payload)
		}
	}
	return nil
}

// Broadcast sends payload to every connected client.
func (h *Hub) Broadcast(payload any) {
	frame, err := json.Marshal(payload)
	if err != nil {
		h.logger.Error("failed to encode broadcast", slog.String("err", err.Error()))
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		client.enqueue(frame)
	}
}

// Each runs fn for every connected client. It is used when each client must
// receive a different frame — a customer only ever sees their own
// conversations, an agent sees the whole queue.
func (h *Hub) Each(fn func(client *Client)) {
	h.mu.RLock()
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.RUnlock()
	// fn runs outside the lock so a handler may itself touch the hub.
	for _, client := range clients {
		fn(client)
	}
}

// Clients reports how many sockets are open, used by the health endpoint.
func (h *Hub) Clients() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Send delivers a payload to a single client.
func (c *Client) Send(payload any) {
	frame, err := json.Marshal(payload)
	if err != nil {
		c.hub.logger.Error("failed to encode frame", slog.String("err", err.Error()))
		return
	}
	c.enqueue(frame)
}

// enqueue drops the frame if the client is not draining its buffer, rather than
// blocking every other subscriber behind one stalled socket.
func (c *Client) enqueue(frame []byte) {
	select {
	case c.send <- frame:
	default:
		c.hub.logger.Warn("dropping frame for slow websocket client", slog.String("user_id", c.UserID))
	}
}

func (c *Client) remove() {
	c.hub.mu.Lock()
	if _, ok := c.hub.clients[c]; ok {
		delete(c.hub.clients, c)
		close(c.send)
	}
	c.hub.mu.Unlock()
}

// readPump drains incoming frames. The clients never send commands over the
// socket (commands go through REST), so this only keeps the connection healthy
// and detects disconnects.
func (c *Client) readPump() {
	defer func() {
		c.remove()
		_ = c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()
	for {
		select {
		case frame, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, frame); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
