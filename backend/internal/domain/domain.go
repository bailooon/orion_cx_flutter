// Package domain holds the entities shared by every Orion CX service.
//
// JSON tags are lowerCamelCase because the Flutter client decodes them
// directly; changing a tag here is a breaking API change.
package domain

import "time"

// Channel is one of the customer-facing entry points unified by Orion (RF008).
type Channel string

const (
	ChannelApp      Channel = "appClaro"
	ChannelWeb      Channel = "webPortal"
	ChannelWhatsApp Channel = "whatsapp"
)

var channelLabels = map[Channel]string{
	ChannelApp:      "App Claro",
	ChannelWeb:      "Web Portal",
	ChannelWhatsApp: "WhatsApp",
}

// Label returns the human readable channel name used in system messages.
func (c Channel) Label() string {
	if label, ok := channelLabels[c]; ok {
		return label
	}
	return string(c)
}

// Valid reports whether c is a channel the platform knows how to serve.
func (c Channel) Valid() bool {
	_, ok := channelLabels[c]
	return ok
}

// Actor identifies who produced a message inside a conversation.
type Actor string

const (
	ActorCustomer  Actor = "customer"
	ActorAssistant Actor = "assistant"
	ActorAgent     Actor = "agent"
	ActorSystem    Actor = "system"
)

// ConversationStatus drives both the customer UI and the agent queue.
type ConversationStatus string

const (
	// StatusBot means the AI is still handling the conversation on its own.
	StatusBot ConversationStatus = "bot"
	// StatusWaitingHuman means a REQUIRED_HUMAN_ASSISTANCE event was published
	// and the conversation is sitting in the agent queue.
	StatusWaitingHuman ConversationStatus = "waitingHuman"
	// StatusInProgress means a human agent owns the conversation.
	StatusInProgress ConversationStatus = "inProgress"
	StatusResolved   ConversationStatus = "resolved"
)

// Pending actions kept in the session context so a journey survives a channel
// switch (RF003).
const (
	ActionRestartSignal = "RESTART_SIGNAL"
	ActionContinue      = "CONTINUE_PENDING_ACTION"
	ActionHumanHandoff  = "REQUIRED_HUMAN_ASSISTANCE"
)

// Role separates customers from the agents who can open the admin dashboard.
type Role string

const (
	RoleCustomer Role = "customer"
	RoleAgent    Role = "agent"
)

// User is owned by the Authenticator service. PasswordHash is never serialised.
type User struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	Name         string     `json:"name"`
	DocumentMask string     `json:"documentMask"`
	PlanName     string     `json:"planName"`
	Role         Role       `json:"role"`
	PasswordHash string     `json:"-"`
	CreatedAt    time.Time  `json:"createdAt"`
	AnonymizedAt *time.Time `json:"anonymizedAt,omitempty"`
}

// Anonymized reports whether the user exercised their LGPD erasure right.
func (u User) Anonymized() bool { return u.AnonymizedAt != nil }

// Message is a single turn in a conversation, always stamped with the channel
// it came from so the history stays auditable across channels (RF002).
type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId"`
	Actor          Actor     `json:"actor"`
	Text           string    `json:"text"`
	SentAt         time.Time `json:"sentAt"`
	Channel        Channel   `json:"channel"`
}

// Conversation is the unit the customer sees as "an attendance" and the agent
// sees as a queue entry. It is owned by the Call Management service.
type Conversation struct {
	ID               string             `json:"id"`
	UserID           string             `json:"userId"`
	CustomerName     string             `json:"customerName"`
	CustomerDocument string             `json:"customerDocument"`
	PlanName         string             `json:"planName"`
	Intent           string             `json:"intent"`
	IntentConfidence float64            `json:"intentConfidence"`
	Summary          string             `json:"summary"`
	Status           ConversationStatus `json:"status"`
	PendingAction    *string            `json:"pendingAction"`
	AssignedAgent    *string            `json:"assignedAgent"`
	HasUnreadEvent   bool               `json:"hasUnreadEvent"`
	CreatedAt        time.Time          `json:"createdAt"`
	UpdatedAt        time.Time          `json:"updatedAt"`
	Messages         []Message          `json:"messages"`
}

// LastChannel is the channel of the most recent message, used to reply to the
// customer on the same channel they last used (flow B, step 6).
func (c Conversation) LastChannel() Channel {
	if len(c.Messages) == 0 {
		return ChannelApp
	}
	return c.Messages[len(c.Messages)-1].Channel
}

// TicketStatus is the lifecycle of a support ticket (RF006).
type TicketStatus string

const (
	TicketOpen       TicketStatus = "open"
	TicketInProgress TicketStatus = "inProgress"
	TicketResolved   TicketStatus = "resolved"
)

// TicketEvent is one entry of a ticket timeline, so the customer can follow
// progress in real time.
type TicketEvent struct {
	At          time.Time `json:"at"`
	Description string    `json:"description"`
}

// Ticket is the protocol number a customer can track.
type Ticket struct {
	ID             string        `json:"id"`
	UserID         string        `json:"userId"`
	ConversationID string        `json:"conversationId"`
	Title          string        `json:"title"`
	Category       string        `json:"category"`
	Status         TicketStatus  `json:"status"`
	Channel        Channel       `json:"channel"`
	CreatedAt      time.Time     `json:"createdAt"`
	UpdatedAt      time.Time     `json:"updatedAt"`
	Timeline       []TicketEvent `json:"timeline"`
}

// Notification is produced by the Notification service from bus events (RF009).
type Notification struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Channel   Channel   `json:"channel"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"createdAt"`
}

// Recommendation is a next-best-action derived from the customer history
// (RF007).
type Recommendation struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	Reason string `json:"reason"`
	Action string `json:"action"`
}

// SessionContext is the low-latency slice of state kept in Redis (DynamoDB in
// production) so any channel can resume a journey (RF003).
type SessionContext struct {
	SessionID      string  `json:"sessionId"`
	UserID         string  `json:"userId"`
	ConversationID string  `json:"conversationId"`
	LastChannel    Channel `json:"lastChannel"`
	PendingAction  string  `json:"pendingAction"`
	LastIntent     string  `json:"lastIntent"`
	// UnclearTurns counts consecutive messages the NLU could not classify. The
	// gateway asks for details on the first one and escalates to a human on the
	// second, instead of bouncing the customer to the queue immediately.
	UnclearTurns int       `json:"unclearTurns"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
