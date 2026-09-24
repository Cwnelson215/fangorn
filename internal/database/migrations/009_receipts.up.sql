-- Receipts: a photographed receipt, what Claude read off it, and the expense it
-- became.
--
-- The photo is kept in the database rather than on disk or in a bucket. The
-- container has no writable volume, and a family's receipts are a few hundred KB
-- each — a bytea column lives in the same Postgres as everything else and is
-- backed up with it.
--
-- Status moves pending -> processing -> posted | needs_review:
--
--   pending       uploaded, waiting for extraction (or backing off after a
--                 transient failure until retry_after)
--   processing    claimed by one worker. The claim is a lease: a worker that
--                 dies mid-call leaves claimed_at behind, and after it expires
--                 the receipt can be claimed again. claim_seq is the fence — a
--                 worker whose lease was taken over finds its claim_seq stale
--                 and its write matches no row.
--   needs_review  held for a person, with review_reasons saying why
--   posted        a transaction exists for it
--
-- There is no "failed". A receipt Claude cannot read still needs the same thing
-- from the user as one it read doubtfully: look at the photo and post it.

ALTER TABLE transactions DROP CONSTRAINT transactions_source_check;
ALTER TABLE transactions ADD CONSTRAINT transactions_source_check
    CHECK (source IN ('manual','recurring','receipt'));

CREATE TABLE receipts (
    id                  SERIAL PRIMARY KEY,
    household_id        INTEGER NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    status              TEXT NOT NULL DEFAULT 'pending',
    review_reasons      TEXT[] NOT NULL DEFAULT '{}',

    image               BYTEA NOT NULL,
    media_type          TEXT NOT NULL,
    byte_size           INTEGER NOT NULL,
    image_sha256        BYTEA NOT NULL,

    -- What Claude read, exactly as read (after cleaning). Kept even once the
    -- transaction is edited: the transaction is the truth, this is the record.
    merchant            TEXT,
    purchased_on        DATE,
    currency            TEXT,
    txn_type            TEXT,
    subtotal            NUMERIC(14,2),
    tax                 NUMERIC(14,2),
    tip                 NUMERIC(14,2),
    total               NUMERIC(14,2),
    tender              TEXT,
    card_last4          TEXT,
    category_suggested  TEXT,
    -- Line items are stored now so a receipt can later be split across
    -- categories without being photographed again.
    line_items          JSONB,
    model               TEXT,

    -- What the suggestion resolved to in this household, if anything.
    account_id          INTEGER REFERENCES accounts(id) ON DELETE SET NULL,
    category_id         INTEGER REFERENCES categories(id) ON DELETE SET NULL,

    claim_seq           INTEGER NOT NULL DEFAULT 0,
    claimed_at          TIMESTAMPTZ,
    failures            INTEGER NOT NULL DEFAULT 0,
    retry_after         TIMESTAMPTZ,
    failed_at           TIMESTAMPTZ,
    extract_error       TEXT,

    -- Deleting the transaction deletes the receipt, the way deleting one leg of a
    -- transfer deletes both. SET NULL would leave a "posted" receipt pointing at
    -- nothing, and the check below rules that state out.
    transaction_id      INTEGER REFERENCES transactions(id) ON DELETE CASCADE,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT receipts_status_check
        CHECK (status IN ('pending','processing','needs_review','posted')),
    CONSTRAINT receipts_posted_has_transaction
        CHECK ((status = 'posted') = (transaction_id IS NOT NULL))
);

-- The same photo uploaded twice (a double tap, a retried upload) is one receipt.
CREATE UNIQUE INDEX idx_receipts_household_sha ON receipts(household_id, image_sha256);
CREATE UNIQUE INDEX idx_receipts_transaction ON receipts(transaction_id)
    WHERE transaction_id IS NOT NULL;
CREATE INDEX idx_receipts_household_status ON receipts(household_id, status, created_at DESC);
