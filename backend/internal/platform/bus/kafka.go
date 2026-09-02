package bus

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"
)

// Kafka is the production-shaped bus. It talks the Kafka protocol, so the same
// code runs against Redpanda locally and Amazon MSK in production.
type Kafka struct {
	broker  string
	writer  *kafka.Writer
	readers []*kafka.Reader
	logger  *slog.Logger
}

// NewKafka connects a producer to broker. Topic auto-creation is enabled so a
// fresh compose stack works without a provisioning step.
func NewKafka(broker string, logger *slog.Logger) *Kafka {
	return &Kafka{
		broker: broker,
		writer: &kafka.Writer{
			Addr:                   kafka.TCP(broker),
			Topic:                  Topic,
			Balancer:               &kafka.Hash{},
			AllowAutoTopicCreation: true,
			WriteTimeout:           5 * time.Second,
			RequiredAcks:           kafka.RequireOne,
		},
		logger: logger,
	}
}

// Publish writes the event keyed by conversation id, which keeps all events of
// one conversation on the same partition and therefore in order.
func (b *Kafka) Publish(ctx context.Context, event Event) error {
	payload, err := Encode(event)
	if err != nil {
		return err
	}
	key := event.ConversationID
	if key == "" {
		key = event.ID
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if err := b.writer.WriteMessages(writeCtx, kafka.Message{Key: []byte(key), Value: payload}); err != nil {
		// A broker hiccup must not fail the customer request that produced the
		// event, so it is logged and swallowed (RNF007). The REST response
		// already carries the authoritative state.
		b.logger.Error("failed to publish event",
			slog.String("type", event.Type),
			slog.String("err", err.Error()),
		)
		return nil
	}
	b.logger.Debug("event published", slog.String("type", event.Type))
	return nil
}

// Subscribe starts a consumer goroutine for the given consumer group.
func (b *Kafka) Subscribe(ctx context.Context, group string, handler Handler) error {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{b.broker},
		Topic:       Topic,
		GroupID:     group,
		MinBytes:    1,
		MaxBytes:    10 << 20,
		MaxWait:     500 * time.Millisecond,
		StartOffset: kafka.LastOffset,
	})
	b.readers = append(b.readers, reader)

	go func() {
		defer func() { _ = reader.Close() }()
		for {
			message, err := reader.FetchMessage(ctx)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return
				}
				b.logger.Warn("kafka fetch failed, retrying",
					slog.String("group", group),
					slog.String("err", err.Error()),
				)
				select {
				case <-ctx.Done():
					return
				case <-time.After(2 * time.Second):
				}
				continue
			}

			event, decodeErr := Decode(message.Value)
			if decodeErr != nil {
				b.logger.Error("discarding malformed event", slog.String("err", decodeErr.Error()))
			} else {
				func() {
					defer func() {
						if recovered := recover(); recovered != nil {
							b.logger.Error("event handler panic", slog.Any("panic", recovered))
						}
					}()
					handlerCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
					defer cancel()
					handler(handlerCtx, event)
				}()
			}
			// Commit even on a malformed payload: replaying it forever would
			// stall the partition.
			if err := reader.CommitMessages(ctx, message); err != nil {
				b.logger.Warn("commit failed", slog.String("err", err.Error()))
			}
		}
	}()
	return nil
}

// Close releases the producer and every consumer.
func (b *Kafka) Close() error {
	for _, reader := range b.readers {
		_ = reader.Close()
	}
	return b.writer.Close()
}
