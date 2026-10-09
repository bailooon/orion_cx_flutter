-- Satisfaction survey, resolution timestamp and AI audit trail.

ALTER TABLE calls.conversations
    ADD COLUMN IF NOT EXISTS resolved_at          TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS satisfaction_score   SMALLINT CHECK (satisfaction_score BETWEEN 1 AND 5),
    ADD COLUMN IF NOT EXISTS satisfaction_comment TEXT,
    ADD COLUMN IF NOT EXISTS rated_at             TIMESTAMPTZ;

-- Dashboards read satisfaction by period; this keeps that scan cheap.
CREATE INDEX IF NOT EXISTS conversations_rated_idx ON calls.conversations (rated_at) WHERE rated_at IS NOT NULL;

-- Every tool call, blocked message, shed request and double-check decision of
-- the AI agents. It is the evidence that the agents only did what they were
-- allowed to do, and it feeds the AI section of the dashboard.
CREATE TABLE IF NOT EXISTS calls.ai_audit (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT        NOT NULL DEFAULT '',
    user_id         TEXT        NOT NULL DEFAULT '',
    agent           TEXT        NOT NULL DEFAULT '',
    kind            TEXT        NOT NULL,
    tool            TEXT        NOT NULL DEFAULT '',
    status          TEXT        NOT NULL DEFAULT '',
    detail          TEXT        NOT NULL DEFAULT '',
    latency_ms      BIGINT      NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ai_audit_created_idx ON calls.ai_audit (created_at DESC);
