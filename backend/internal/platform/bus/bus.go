// Package bus is the asynchronous event backbone between Orion services.
//
// Production uses Apache Kafka (Amazon MSK). The prototype ships two
// interchangeable implementations behind the same interface: Kafka/Redpanda
// when ORION_KAFKA_BROKER is set, and an in-process fan-out bus otherwise, so
// the whole platform still runs with zero external dependencies.
package bus

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Topic is the single stream every domain event flows through. One topic keeps
// ordering per conversation simple; in production it would be partitioned by
// conversation id.
const Topic = "orion.events"

// Event types published by the services.
const (
	// EventHumanAssistanceRequired is the flow B trigger: NLU confidence fell
	// below the threshold and a human must take over.
	EventHumanAssistanceRequired = "REQUIRED_HUMAN_ASSISTANCE"
	EventConversationUpdated     = "CONVERSATION_UPDATED"
	EventConversationResolved    = "CONVERSATION_RESOLVED"
	EventAgentAssigned           = "AGENT_ASSIGNED"
	EventAgentReplied            = "AGENT_REPLIED"
	EventTicketCreated           = "TICKET_CREATED"
	EventTicketUpdated           = "TICKET_UPDATED"
)

// Event is the envelope carried by the bus.
type Event struct {
	ID             string         `json:"id"`
	Type           string         `json:"type"`
	UserID         string         `json:"userId,omitempty"`
	ConversationID string         `json:"conversationId,omitempty"`
	TicketID       string         `json:"ticketId,omitempty"`
	Channel        string         `json:"channel,omitempty"`
	Payload        map[string]any `json:"payload,omitempty"`
	OccurredAt     time.Time      `json:"occurredAt"`
}

// NewEvent stamps an id and timestamp on a partially filled event.
func NewEvent(eventType string) Event {
	return Event{ID: uuid.NewString(), Type: eventType, OccurredAt: time.Now().UTC()}
}

// Handler consumes one event. It must not block for long: the bus delivers
// events sequentially per subscriber.
type Handler func(context.Context, Event)

// Bus is the contract both implementations satisfy.
type Bus interface {
	// Publish emits an event. It never blocks the caller on a slow consumer.
	Publish(ctx context.Context, event Event) error
	// Subscribe registers a handler. group identifies the logical consumer so
	// Kafka can balance partitions; the in-process bus fans out to everyone.
	Subscribe(ctx context.Context, group string, handler Handler) error
	Close() error
}

// InProcess is the dependency-free bus used when Kafka is not configured. It
// only reaches subscribers inside the same process, which is exactly the
// `-service=all` topology.
type InProcess struct {
	mu          sync.RWMutex
	subscribers []Handler
	logger      *slog.Logger
	closed      bool
}

// NewInProcess builds the fallback bus.
func NewInProcess(logger *slog.Logger) *InProcess {
	return &InProcess{logger: logger}
}

// Publish delivers the event to every subscriber on its own goroutine, so one
// slow handler cannot delay the HTTP request that produced the event.
func (b *InProcess) Publish(ctx context.Context, event Event) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return nil
	}
	b.logger.Debug("event published",
		slog.String("type", event.Type),
		slog.String("conversation_id", event.ConversationID),
	)
	for _, handler := range b.subscribers {
		go func(h Handler) {
			defer func() {
				if recovered := recover(); recovered != nil {
					b.logger.Error("event handler panic", slog.Any("panic", recovered))
				}
			}()
			// The publisher context dies with its request; consumers get a
			// fresh bounded context instead.
			consumerCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			h(consumerCtx, event)
		}(handler)
	}
	return nil
}

// Subscribe registers a handler for every future event.
func (b *InProcess) Subscribe(_ context.Context, _ string, handler Handler) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers = append(b.subscribers, handler)
	return nil
}

// Close stops further delivery.
func (b *InProcess) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	b.subscribers = nil
	return nil
}

// Encode/Decode are shared by the Kafka implementation and by tests.
func Encode(event Event) ([]byte, error) { return json.Marshal(event) }

// Decode parses an event payload coming off the wire.
func Decode(raw []byte) (Event, error) {
	var event Event
	err := json.Unmarshal(raw, &event)
	return event, err
}
