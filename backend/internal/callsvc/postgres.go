package callsvc

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/orion-cx/orion-backend/internal/domain"
	"github.com/orion-cx/orion-backend/internal/platform/httpx"
)

// PostgresRepository persists conversations, messages and tickets in the calls
// schema. Every multi-row write goes through a transaction so a partial turn
// can never be observed.
type PostgresRepository struct{ pool *pgxpool.Pool }

// NewPostgresRepository wires the repository to a pool.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const conversationColumns = `id, user_id, customer_name, customer_document, plan_name,
	intent, intent_confidence, summary, status, pending_action, assigned_agent,
	has_unread_event, created_at, updated_at`

func scanConversation(row pgx.Row) (domain.Conversation, error) {
	var conversation domain.Conversation
	err := row.Scan(
		&conversation.ID, &conversation.UserID, &conversation.CustomerName,
		&conversation.CustomerDocument, &conversation.PlanName, &conversation.Intent,
		&conversation.IntentConfidence, &conversation.Summary, &conversation.Status,
		&conversation.PendingAction, &conversation.AssignedAgent,
		&conversation.HasUnreadEvent, &conversation.CreatedAt, &conversation.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Conversation{}, httpx.ErrNotFound
	}
	conversation.Messages = []domain.Message{}
	return conversation, err
}

// CreateConversation inserts a conversation row.
func (r *PostgresRepository) CreateConversation(ctx context.Context, conversation domain.Conversation) (domain.Conversation, error) {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO calls.conversations (`+conversationColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		conversation.ID, conversation.UserID, conversation.CustomerName,
		conversation.CustomerDocument, conversation.PlanName, conversation.Intent,
		conversation.IntentConfidence, conversation.Summary, conversation.Status,
		conversation.PendingAction, conversation.AssignedAgent,
		conversation.HasUnreadEvent, conversation.CreatedAt, conversation.UpdatedAt,
	)
	if err != nil {
		return domain.Conversation{}, err
	}
	return r.GetConversation(ctx, conversation.ID)
}

// GetConversation loads a conversation and its ordered history.
func (r *PostgresRepository) GetConversation(ctx context.Context, id string) (domain.Conversation, error) {
	conversation, err := scanConversation(r.pool.QueryRow(ctx,
		`SELECT `+conversationColumns+` FROM calls.conversations WHERE id = $1`, id))
	if err != nil {
		return domain.Conversation{}, err
	}
	messages, err := r.messagesFor(ctx, []string{id})
	if err != nil {
		return domain.Conversation{}, err
	}
	conversation.Messages = messages[id]
	if conversation.Messages == nil {
		conversation.Messages = []domain.Message{}
	}
	return conversation, nil
}

// ListConversations loads conversations plus their histories using two queries
// instead of one per conversation, which keeps the dashboard fast (RNF001).
func (r *PostgresRepository) ListConversations(ctx context.Context, filter ConversationFilter) ([]domain.Conversation, error) {
	query := `SELECT ` + conversationColumns + ` FROM calls.conversations WHERE 1 = 1`
	args := make([]any, 0, 2)
	if filter.UserID != "" {
		args = append(args, filter.UserID)
		query += ` AND user_id = $1`
	}
	if len(filter.Statuses) > 0 {
		statuses := make([]string, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			statuses = append(statuses, string(status))
		}
		args = append(args, statuses)
		if len(args) == 1 {
			query += ` AND status = ANY($1)`
		} else {
			query += ` AND status = ANY($2)`
		}
	}
	query += ` ORDER BY updated_at`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	conversations := make([]domain.Conversation, 0)
	ids := make([]string, 0)
	for rows.Next() {
		conversation, err := scanConversation(rows)
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, conversation)
		ids = append(ids, conversation.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return conversations, nil
	}

	messages, err := r.messagesFor(ctx, ids)
	if err != nil {
		return nil, err
	}
	for index := range conversations {
		if history, ok := messages[conversations[index].ID]; ok {
			conversations[index].Messages = history
		}
	}
	return conversations, nil
}

func (r *PostgresRepository) messagesFor(ctx context.Context, ids []string) (map[string][]domain.Message, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, conversation_id, actor, body, channel, sent_at
		  FROM calls.messages
		 WHERE conversation_id = ANY($1)
		 ORDER BY sent_at, id`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grouped := make(map[string][]domain.Message, len(ids))
	for rows.Next() {
		var message domain.Message
		if err := rows.Scan(&message.ID, &message.ConversationID, &message.Actor,
			&message.Text, &message.Channel, &message.SentAt); err != nil {
			return nil, err
		}
		grouped[message.ConversationID] = append(grouped[message.ConversationID], message)
	}
	return grouped, rows.Err()
}

// ApplyTurn writes the messages and the new conversation state atomically.
func (r *PostgresRepository) ApplyTurn(ctx context.Context, id string, turn Turn) (domain.Conversation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Conversation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the row so two concurrent turns on the same conversation serialise
	// instead of interleaving their state updates.
	current, err := scanConversation(tx.QueryRow(ctx,
		`SELECT `+conversationColumns+` FROM calls.conversations WHERE id = $1 FOR UPDATE`, id))
	if err != nil {
		return domain.Conversation{}, err
	}

	now := time.Now().UTC()
	for index, message := range turn.Messages {
		if _, err := tx.Exec(ctx, `
			INSERT INTO calls.messages (id, conversation_id, actor, body, channel, sent_at)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			"MSG-"+uuid.NewString()[:8], id, message.Actor, message.Text, message.Channel,
			now.Add(time.Duration(index)*time.Millisecond),
		); err != nil {
			return domain.Conversation{}, err
		}
	}

	applyTurnFields(&current, turn)
	current.UpdatedAt = now

	if _, err := tx.Exec(ctx, `
		UPDATE calls.conversations
		   SET intent = $2, intent_confidence = $3, summary = $4, status = $5,
		       pending_action = $6, assigned_agent = $7, has_unread_event = $8,
		       updated_at = $9
		 WHERE id = $1`,
		id, current.Intent, current.IntentConfidence, current.Summary, current.Status,
		current.PendingAction, current.AssignedAgent, current.HasUnreadEvent, current.UpdatedAt,
	); err != nil {
		return domain.Conversation{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Conversation{}, err
	}
	return r.GetConversation(ctx, id)
}

// ResetConversation clears history and classification, used by the demo reset.
func (r *PostgresRepository) ResetConversation(ctx context.Context, id string) (domain.Conversation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Conversation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM calls.messages WHERE conversation_id = $1`, id); err != nil {
		return domain.Conversation{}, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE calls.conversations
		   SET intent = $2, intent_confidence = 0, summary = $3, status = 'bot',
		       pending_action = NULL, assigned_agent = NULL, has_unread_event = FALSE,
		       updated_at = now()
		 WHERE id = $1`, id, IntentUnclassified, SummaryNewSession)
	if err != nil {
		return domain.Conversation{}, err
	}
	if tag.RowsAffected() == 0 {
		return domain.Conversation{}, httpx.ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Conversation{}, err
	}
	return r.GetConversation(ctx, id)
}

// CountConversations reports the table size, used by the seeder.
func (r *PostgresRepository) CountConversations(ctx context.Context) (int, error) {
	var total int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM calls.conversations`).Scan(&total)
	return total, err
}

// CreateTicket opens a ticket together with its first timeline entry.
func (r *PostgresRepository) CreateTicket(ctx context.Context, input TicketInput) (domain.Ticket, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Ticket{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		INSERT INTO calls.tickets (id, user_id, conversation_id, title, category, status, channel, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)`,
		input.ID, input.UserID, input.ConversationID, input.Title, input.Category,
		input.Status, input.Channel, now,
	); err != nil {
		return domain.Ticket{}, err
	}
	if input.FirstEvent != "" {
		if _, err := tx.Exec(ctx,
			`INSERT INTO calls.ticket_events (ticket_id, description, at) VALUES ($1,$2,$3)`,
			input.ID, input.FirstEvent, now,
		); err != nil {
			return domain.Ticket{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Ticket{}, err
	}
	return r.GetTicket(ctx, input.ID)
}

const ticketColumns = `id, user_id, conversation_id, title, category, status, channel, created_at, updated_at`

func scanTicket(row pgx.Row) (domain.Ticket, error) {
	var ticket domain.Ticket
	err := row.Scan(&ticket.ID, &ticket.UserID, &ticket.ConversationID, &ticket.Title,
		&ticket.Category, &ticket.Status, &ticket.Channel, &ticket.CreatedAt, &ticket.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Ticket{}, httpx.ErrNotFound
	}
	ticket.Timeline = []domain.TicketEvent{}
	return ticket, err
}

// GetTicket loads a ticket with its timeline.
func (r *PostgresRepository) GetTicket(ctx context.Context, id string) (domain.Ticket, error) {
	ticket, err := scanTicket(r.pool.QueryRow(ctx,
		`SELECT `+ticketColumns+` FROM calls.tickets WHERE id = $1`, id))
	if err != nil {
		return domain.Ticket{}, err
	}
	timeline, err := r.timelineFor(ctx, []string{id})
	if err != nil {
		return domain.Ticket{}, err
	}
	if events, ok := timeline[id]; ok {
		ticket.Timeline = events
	}
	return ticket, nil
}

// ListTickets returns a customer's protocols, newest first.
func (r *PostgresRepository) ListTickets(ctx context.Context, userID string) ([]domain.Ticket, error) {
	if userID == "" {
		return r.queryTickets(ctx, `SELECT `+ticketColumns+` FROM calls.tickets ORDER BY created_at DESC`)
	}
	return r.queryTickets(ctx,
		`SELECT `+ticketColumns+` FROM calls.tickets WHERE user_id = $1 ORDER BY created_at DESC`, userID)
}

// TicketsByConversation returns the protocols linked to a conversation.
func (r *PostgresRepository) TicketsByConversation(ctx context.Context, conversationID string) ([]domain.Ticket, error) {
	return r.queryTickets(ctx,
		`SELECT `+ticketColumns+` FROM calls.tickets WHERE conversation_id = $1 ORDER BY created_at DESC`, conversationID)
}

func (r *PostgresRepository) queryTickets(ctx context.Context, query string, args ...any) ([]domain.Ticket, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tickets := make([]domain.Ticket, 0)
	ids := make([]string, 0)
	for rows.Next() {
		ticket, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, ticket)
		ids = append(ids, ticket.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return tickets, nil
	}
	timeline, err := r.timelineFor(ctx, ids)
	if err != nil {
		return nil, err
	}
	for index := range tickets {
		if events, ok := timeline[tickets[index].ID]; ok {
			tickets[index].Timeline = events
		}
	}
	return tickets, nil
}

func (r *PostgresRepository) timelineFor(ctx context.Context, ids []string) (map[string][]domain.TicketEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ticket_id, description, at
		  FROM calls.ticket_events
		 WHERE ticket_id = ANY($1)
		 ORDER BY at, id`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grouped := make(map[string][]domain.TicketEvent, len(ids))
	for rows.Next() {
		var ticketID string
		var event domain.TicketEvent
		if err := rows.Scan(&ticketID, &event.Description, &event.At); err != nil {
			return nil, err
		}
		grouped[ticketID] = append(grouped[ticketID], event)
	}
	return grouped, rows.Err()
}

// UpdateTicket advances a ticket and appends a timeline entry.
func (r *PostgresRepository) UpdateTicket(ctx context.Context, id string, update TicketUpdate) (domain.Ticket, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Ticket{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().UTC()
	if update.Status != nil {
		tag, err := tx.Exec(ctx,
			`UPDATE calls.tickets SET status = $2, updated_at = $3 WHERE id = $1`, id, *update.Status, now)
		if err != nil {
			return domain.Ticket{}, err
		}
		if tag.RowsAffected() == 0 {
			return domain.Ticket{}, httpx.ErrNotFound
		}
	} else {
		tag, err := tx.Exec(ctx, `UPDATE calls.tickets SET updated_at = $2 WHERE id = $1`, id, now)
		if err != nil {
			return domain.Ticket{}, err
		}
		if tag.RowsAffected() == 0 {
			return domain.Ticket{}, httpx.ErrNotFound
		}
	}
	if update.Event != "" {
		if _, err := tx.Exec(ctx,
			`INSERT INTO calls.ticket_events (ticket_id, description, at) VALUES ($1,$2,$3)`,
			id, update.Event, now,
		); err != nil {
			return domain.Ticket{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Ticket{}, err
	}
	return r.GetTicket(ctx, id)
}

// PurgeUser deletes every conversation, message and ticket of a user. Messages
// and ticket events go with their parents through ON DELETE CASCADE.
func (r *PostgresRepository) PurgeUser(ctx context.Context, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM calls.tickets WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM calls.conversations WHERE user_id = $1`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Ping backs the readiness probe.
func (r *PostgresRepository) Ping(ctx context.Context) error { return r.pool.Ping(ctx) }
