-- Conversations with the assistant.
--
-- messages is the Messages API transcript exactly as it was sent and received:
-- user turns, assistant turns with their thinking and tool_use blocks, and the
-- tool results. It is replayed verbatim on the next turn, which is what the API
-- needs (thinking blocks are only valid unchanged, in the conversation that
-- produced them), so it is only ever appended to. What the app shows is derived
-- from it on read.
CREATE TABLE chats (
    id           SERIAL PRIMARY KEY,
    household_id INTEGER NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    title        TEXT NOT NULL DEFAULT '',
    messages     JSONB NOT NULL DEFAULT '[]',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chats_messages_array CHECK (jsonb_typeof(messages) = 'array')
);

CREATE INDEX chats_household_updated ON chats (household_id, updated_at DESC);
