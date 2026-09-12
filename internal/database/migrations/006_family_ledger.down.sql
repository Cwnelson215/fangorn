-- Reverse 006: drop the family-ledger schema and rebuild the post-005 bank-sync
-- schema. Structure only — the ledger data is not convertible back into Teller/CSV
-- rows and is dropped with the tables.

DROP TABLE IF EXISTS net_worth_snapshots CASCADE;
DROP TABLE IF EXISTS goal_contributions CASCADE;
DROP TABLE IF EXISTS goals CASCADE;
DROP TABLE IF EXISTS budgets CASCADE;
DROP TABLE IF EXISTS recurring_occurrences CASCADE;
DROP TABLE IF EXISTS transactions CASCADE;
DROP TABLE IF EXISTS recurring_rules CASCADE;
DROP TABLE IF EXISTS categories CASCADE;
DROP TABLE IF EXISTS accounts CASCADE;
DROP TABLE IF EXISTS households CASCADE;

CREATE TABLE linked_institutions (
    id SERIAL PRIMARY KEY,
    institution_id TEXT,
    institution_name TEXT,
    encrypted_access_token TEXT NOT NULL,
    last_synced_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE accounts (
    id SERIAL PRIMARY KEY,
    linked_institution_id INTEGER NOT NULL REFERENCES linked_institutions(id) ON DELETE CASCADE,
    external_account_id TEXT,
    name TEXT NOT NULL,
    official_name TEXT,
    type TEXT NOT NULL,
    subtype TEXT,
    mask TEXT,
    current_balance DECIMAL(12,2),
    available_balance DECIMAL(12,2),
    iso_currency_code TEXT NOT NULL DEFAULT 'USD',
    source TEXT NOT NULL DEFAULT 'teller',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_accounts_linked_institution_id ON accounts(linked_institution_id);
CREATE UNIQUE INDEX idx_accounts_external_account_id ON accounts(external_account_id)
    WHERE external_account_id IS NOT NULL;

CREATE TABLE transactions (
    id SERIAL PRIMARY KEY,
    external_id TEXT,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    amount DECIMAL(12,2) NOT NULL,
    iso_currency_code TEXT NOT NULL DEFAULT 'USD',
    date DATE NOT NULL,
    name TEXT NOT NULL,
    merchant_name TEXT,
    category TEXT,
    pending BOOLEAN NOT NULL DEFAULT FALSE,
    source TEXT NOT NULL DEFAULT 'teller',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_transactions_date ON transactions(date);
CREATE INDEX idx_transactions_account_id ON transactions(account_id);
CREATE UNIQUE INDEX idx_transactions_external_id ON transactions(external_id)
    WHERE external_id IS NOT NULL;

CREATE TABLE net_worth_snapshots (
    id SERIAL PRIMARY KEY,
    total_assets DECIMAL(12,2) NOT NULL,
    total_liabilities DECIMAL(12,2) NOT NULL,
    net_worth DECIMAL(12,2) NOT NULL,
    snapshot_date DATE NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE transfers (
    id SERIAL PRIMARY KEY,
    source_account_id INTEGER NOT NULL REFERENCES accounts(id),
    destination_account_id INTEGER NOT NULL REFERENCES accounts(id),
    amount DECIMAL(12,2) NOT NULL,
    description TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    teller_transfer_id TEXT,
    failure_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_transfers_status ON transfers(status);
CREATE INDEX idx_transfers_created_at ON transfers(created_at);

CREATE TABLE csv_imports (
    id SERIAL PRIMARY KEY,
    bank_name TEXT NOT NULL,
    file_name TEXT NOT NULL,
    rows_imported INTEGER NOT NULL DEFAULT 0,
    rows_skipped INTEGER NOT NULL DEFAULT 0,
    account_id INTEGER REFERENCES accounts(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE gmail_watch_state (
    id SERIAL PRIMARY KEY,
    last_history_id TEXT,
    last_polled_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE bank_csv_formats (
    id SERIAL PRIMARY KEY,
    bank_name TEXT NOT NULL UNIQUE,
    date_column TEXT NOT NULL,
    amount_column TEXT NOT NULL,
    description_column TEXT NOT NULL,
    category_column TEXT,
    negate_amounts BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
