-- Rename teller-specific columns to generic names
ALTER TABLE transactions RENAME COLUMN teller_transaction_id TO external_id;
ALTER TABLE transactions ALTER COLUMN external_id DROP NOT NULL;
ALTER TABLE transactions ADD COLUMN source TEXT NOT NULL DEFAULT 'teller';

-- Replace unique constraint with partial unique index
DROP INDEX IF EXISTS transactions_teller_transaction_id_key;
CREATE UNIQUE INDEX idx_transactions_external_id ON transactions(external_id) WHERE external_id IS NOT NULL;

ALTER TABLE accounts RENAME COLUMN teller_account_id TO external_account_id;
ALTER TABLE accounts ALTER COLUMN external_account_id DROP NOT NULL;
ALTER TABLE accounts ADD COLUMN source TEXT NOT NULL DEFAULT 'teller';

DROP INDEX IF EXISTS accounts_teller_account_id_key;
CREATE UNIQUE INDEX idx_accounts_external_account_id ON accounts(external_account_id) WHERE external_account_id IS NOT NULL;

-- CSV import tracking
CREATE TABLE IF NOT EXISTS csv_imports (
    id SERIAL PRIMARY KEY,
    bank_name TEXT NOT NULL,
    file_name TEXT NOT NULL,
    rows_imported INTEGER NOT NULL DEFAULT 0,
    rows_skipped INTEGER NOT NULL DEFAULT 0,
    account_id INTEGER REFERENCES accounts(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Gmail watcher state
CREATE TABLE IF NOT EXISTS gmail_watch_state (
    id SERIAL PRIMARY KEY,
    last_history_id TEXT,
    last_polled_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
