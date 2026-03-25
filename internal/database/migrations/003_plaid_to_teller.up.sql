-- Rename plaid_items table to linked_institutions
ALTER TABLE plaid_items RENAME TO linked_institutions;

-- Remove cursor column (Teller doesn't use cursors), add last_synced_at
ALTER TABLE linked_institutions DROP COLUMN IF EXISTS cursor;
ALTER TABLE linked_institutions ADD COLUMN last_synced_at TIMESTAMPTZ;

-- Rename plaid-specific columns in accounts
ALTER TABLE accounts RENAME COLUMN plaid_item_id TO linked_institution_id;
ALTER TABLE accounts RENAME COLUMN plaid_account_id TO teller_account_id;

-- Rename index
DROP INDEX IF EXISTS idx_accounts_plaid_item_id;
CREATE INDEX idx_accounts_linked_institution_id ON accounts(linked_institution_id);

-- Rename plaid-specific columns in transactions
ALTER TABLE transactions RENAME COLUMN plaid_transaction_id TO teller_transaction_id;

-- Drop plaid_category, keep category
ALTER TABLE transactions DROP COLUMN IF EXISTS plaid_category;

-- Simplify transfers table: remove Plaid-specific dual-leg columns
ALTER TABLE transfers DROP COLUMN IF EXISTS debit_plaid_transfer_id;
ALTER TABLE transfers DROP COLUMN IF EXISTS credit_plaid_transfer_id;
ALTER TABLE transfers DROP COLUMN IF EXISTS debit_authorization_id;
ALTER TABLE transfers DROP COLUMN IF EXISTS credit_authorization_id;
ALTER TABLE transfers DROP COLUMN IF EXISTS debit_status;
ALTER TABLE transfers DROP COLUMN IF EXISTS credit_status;

-- Add single Teller transfer tracking
ALTER TABLE transfers ADD COLUMN teller_transfer_id TEXT;
