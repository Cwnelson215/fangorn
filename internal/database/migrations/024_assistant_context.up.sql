-- What the assistant knows beyond the ledger: the family's own instructions
-- (who they are, their plans, how they want to be helped) and files they've
-- attached — the same things a claude.ai Project holds.
ALTER TABLE households ADD COLUMN assistant_instructions TEXT NOT NULL DEFAULT '';

-- A removed file that a chat still uses keeps its bytes (removed_at set): a
-- chat's context is fixed when it starts, because changing what came before
-- its first question would rewrite history the model already answered from.
CREATE TABLE assistant_files (
    id           SERIAL PRIMARY KEY,
    household_id INTEGER NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    media_type   TEXT NOT NULL CHECK (media_type IN ('application/pdf', 'text/plain')),
    data         BYTEA NOT NULL,
    size         INTEGER NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    removed_at   TIMESTAMPTZ
);

CREATE INDEX assistant_files_household ON assistant_files (household_id) WHERE removed_at IS NULL;

-- The instructions and file ids a chat started with: {"instructions": "...",
-- "file_ids": [1, 2]}. NULL for chats from before this existed, which go on
-- without any.
ALTER TABLE chats ADD COLUMN context JSONB;
