-- Migration 004: Conversation logging (S3-7)
-- Records every chatbot conversation and all its messages.
-- Used to track user interactions and audit assistant responses.

-- ---------------------------------------------------------------------
-- conversations
-- One record per chatbot session.
-- message_count is updated after each new message is inserted.
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS conversations (
    conversation_id BIGSERIAL   PRIMARY KEY,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    message_count   INTEGER     NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_conversations_started_at  ON conversations (started_at);

-- ---------------------------------------------------------------------
-- messages
-- One record per message in a conversation (1:N with conversations).
-- role must be 'user' or 'assistant' — no other values are allowed.
-- agent is NULL for user messages.
-- agent holds the handler name for assistant messages.
-- ON DELETE CASCADE: messages have no meaning without their conversation.
-- ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS messages (
    message_id      BIGSERIAL   PRIMARY KEY,
    conversation_id BIGINT      NOT NULL REFERENCES conversations (conversation_id) ON DELETE CASCADE,
    role            TEXT        NOT NULL CHECK (role IN ('user', 'assistant')),
    content         TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    agent           TEXT
);

CREATE INDEX IF NOT EXISTS idx_messages_conversation_id  ON messages (conversation_id);
