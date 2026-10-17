-- Orion CX initial schema.
--
-- Each service owns its own schema and never reads another one: the Call
-- Management tables keep a denormalised copy of the customer display data
-- instead of joining Authenticator tables. That is what allows the services to
-- be split into separate databases in production without touching queries.

CREATE SCHEMA IF NOT EXISTS auth;
CREATE SCHEMA IF NOT EXISTS calls;
CREATE SCHEMA IF NOT EXISTS notify;

-- ORION Authenticator ---------------------------------------------------------
CREATE TABLE IF NOT EXISTS auth.users (
    id             TEXT PRIMARY KEY,
    email          TEXT        NOT NULL UNIQUE,
    name           TEXT        NOT NULL,
    document_mask  TEXT        NOT NULL DEFAULT '',
    plan_name      TEXT        NOT NULL DEFAULT '',
    role           TEXT        NOT NULL CHECK (role IN ('customer', 'agent')),
    password_hash  TEXT        NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Set when the user exercises their LGPD erasure right. The row is kept so
    -- ticket protocols stay auditable, but every identifying column is wiped.
    anonymized_at  TIMESTAMPTZ
);

-- ORION Call Management -------------------------------------------------------
CREATE TABLE IF NOT EXISTS calls.conversations (
    id                TEXT PRIMARY KEY,
    user_id           TEXT        NOT NULL,
    customer_name     TEXT        NOT NULL DEFAULT '',
    customer_document TEXT        NOT NULL DEFAULT '',
    plan_name         TEXT        NOT NULL DEFAULT '',
    intent            TEXT        NOT NULL DEFAULT 'NAO_CLASSIFICADA',
    intent_confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    summary           TEXT        NOT NULL DEFAULT '',
    status            TEXT        NOT NULL CHECK (status IN ('bot', 'waitingHuman', 'inProgress', 'resolved')),
    pending_action    TEXT,
    assigned_agent    TEXT,
    has_unread_event  BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS conversations_user_idx   ON calls.conversations (user_id);
-- The agent queue reads by status ordered by age; this index keeps it cheap as
-- the table grows (RNF003).
CREATE INDEX IF NOT EXISTS conversations_status_idx ON calls.conversations (status, updated_at);

CREATE TABLE IF NOT EXISTS calls.messages (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT        NOT NULL REFERENCES calls.conversations (id) ON DELETE CASCADE,
    actor           TEXT        NOT NULL CHECK (actor IN ('customer', 'assistant', 'agent', 'system')),
    body            TEXT        NOT NULL,
    channel         TEXT        NOT NULL,
    sent_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- History is always read as "one conversation, oldest first" (RF002).
CREATE INDEX IF NOT EXISTS messages_conversation_idx ON calls.messages (conversation_id, sent_at);

CREATE TABLE IF NOT EXISTS calls.tickets (
    id              TEXT PRIMARY KEY,
    user_id         TEXT        NOT NULL,
    conversation_id TEXT        NOT NULL,
    title           TEXT        NOT NULL,
    category        TEXT        NOT NULL,
    status          TEXT        NOT NULL CHECK (status IN ('open', 'inProgress', 'resolved')),
    channel         TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS tickets_user_idx         ON calls.tickets (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS tickets_conversation_idx ON calls.tickets (conversation_id);

CREATE TABLE IF NOT EXISTS calls.ticket_events (
    id          BIGSERIAL PRIMARY KEY,
    ticket_id   TEXT        NOT NULL REFERENCES calls.tickets (id) ON DELETE CASCADE,
    description TEXT        NOT NULL,
    at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ticket_events_ticket_idx ON calls.ticket_events (ticket_id, at);

-- ORION Notification ----------------------------------------------------------
CREATE TABLE IF NOT EXISTS notify.notifications (
    id         TEXT PRIMARY KEY,
    user_id    TEXT        NOT NULL,
    title      TEXT        NOT NULL,
    body       TEXT        NOT NULL,
    channel    TEXT        NOT NULL,
    read       BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS notifications_user_idx ON notify.notifications (user_id, created_at DESC);
