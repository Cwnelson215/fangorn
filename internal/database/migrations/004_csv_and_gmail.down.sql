DROP TABLE IF EXISTS gmail_watch_state;
DROP TABLE IF EXISTS csv_imports;

ALTER TABLE accounts DROP COLUMN IF EXISTS source;
DROP INDEX IF EXISTS idx_accounts_external_account_id;
ALTER TABLE accounts RENAME COLUMN external_account_id TO teller_account_id;
ALTER TABLE accounts ALTER COLUMN teller_account_id SET NOT NULL;
CREATE UNIQUE INDEX accounts_teller_account_id_key ON accounts(teller_account_id);

ALTER TABLE transactions DROP COLUMN IF EXISTS source;
DROP INDEX IF EXISTS idx_transactions_external_id;
ALTER TABLE transactions RENAME COLUMN external_id TO teller_transaction_id;
ALTER TABLE transactions ALTER COLUMN teller_transaction_id SET NOT NULL;
CREATE UNIQUE INDEX transactions_teller_transaction_id_key ON transactions(teller_transaction_id);
