-- Remove teller transfer column
ALTER TABLE transfers DROP COLUMN IF EXISTS teller_transfer_id;

-- Restore Plaid-specific dual-leg columns
ALTER TABLE transfers ADD COLUMN debit_plaid_transfer_id TEXT;
ALTER TABLE transfers ADD COLUMN credit_plaid_transfer_id TEXT;
ALTER TABLE transfers ADD COLUMN debit_authorization_id TEXT;
ALTER TABLE transfers ADD COLUMN credit_authorization_id TEXT;
ALTER TABLE transfers ADD COLUMN debit_status TEXT;
ALTER TABLE transfers ADD COLUMN credit_status TEXT;

-- Restore plaid_category column
ALTER TABLE transactions ADD COLUMN plaid_category TEXT;

-- Rename transaction column back
ALTER TABLE transactions RENAME COLUMN teller_transaction_id TO plaid_transaction_id;

-- Rename account columns back
DROP INDEX IF EXISTS idx_accounts_linked_institution_id;
ALTER TABLE accounts RENAME COLUMN linked_institution_id TO plaid_item_id;
ALTER TABLE accounts RENAME COLUMN teller_account_id TO plaid_account_id;
CREATE INDEX idx_accounts_plaid_item_id ON accounts(plaid_item_id);

-- Restore cursor column, remove last_synced_at
ALTER TABLE linked_institutions DROP COLUMN IF EXISTS last_synced_at;
ALTER TABLE linked_institutions ADD COLUMN cursor TEXT;

-- Rename table back
ALTER TABLE linked_institutions RENAME TO plaid_items;
