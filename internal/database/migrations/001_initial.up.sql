CREATE TABLE IF NOT EXISTS plaid_items (
    id SERIAL PRIMARY KEY,
    institution_id TEXT,
    institution_name TEXT,
    encrypted_access_token TEXT NOT NULL,
    cursor TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS accounts (
    id SERIAL PRIMARY KEY,
    plaid_item_id INTEGER NOT NULL REFERENCES plaid_items(id) ON DELETE CASCADE,
    plaid_account_id TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    official_name TEXT,
    type TEXT NOT NULL,
    subtype TEXT,
    mask TEXT,
    current_balance DECIMAL(12,2),
    available_balance DECIMAL(12,2),
    iso_currency_code TEXT NOT NULL DEFAULT 'USD',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_accounts_plaid_item_id ON accounts(plaid_item_id);

CREATE TABLE IF NOT EXISTS transactions (
    id SERIAL PRIMARY KEY,
    plaid_transaction_id TEXT UNIQUE NOT NULL,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    amount DECIMAL(12,2) NOT NULL,
    iso_currency_code TEXT NOT NULL DEFAULT 'USD',
    date DATE NOT NULL,
    name TEXT NOT NULL,
    merchant_name TEXT,
    category TEXT,
    plaid_category TEXT,
    pending BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_transactions_date ON transactions(date);
CREATE INDEX idx_transactions_account_id ON transactions(account_id);

CREATE TABLE IF NOT EXISTS net_worth_snapshots (
    id SERIAL PRIMARY KEY,
    total_assets DECIMAL(12,2) NOT NULL,
    total_liabilities DECIMAL(12,2) NOT NULL,
    net_worth DECIMAL(12,2) NOT NULL,
    snapshot_date DATE NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
