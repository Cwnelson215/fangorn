CREATE TABLE IF NOT EXISTS transfers (
    id SERIAL PRIMARY KEY,
    source_account_id INTEGER NOT NULL REFERENCES accounts(id),
    destination_account_id INTEGER NOT NULL REFERENCES accounts(id),
    amount DECIMAL(12,2) NOT NULL,
    description TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    debit_plaid_transfer_id TEXT,
    credit_plaid_transfer_id TEXT,
    debit_authorization_id TEXT,
    credit_authorization_id TEXT,
    debit_status TEXT,
    credit_status TEXT,
    failure_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_transfers_status ON transfers(status);
CREATE INDEX idx_transfers_created_at ON transfers(created_at);
