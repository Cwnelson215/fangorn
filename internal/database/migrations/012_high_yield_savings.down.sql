-- Interest already posted stays on the ledger as manual income; high-yield
-- savings accounts become plain savings accounts.
UPDATE transactions SET source = 'manual' WHERE source = 'interest';
ALTER TABLE transactions DROP CONSTRAINT transactions_source_check;
ALTER TABLE transactions ADD CONSTRAINT transactions_source_check
    CHECK (source IN ('manual','recurring','receipt'));

DROP TABLE IF EXISTS interest_postings;
DROP TABLE IF EXISTS savings_rates;

UPDATE accounts SET type = 'savings' WHERE type = 'high_yield_savings';

ALTER TABLE accounts DROP CONSTRAINT accounts_class_matches_type;
ALTER TABLE accounts ADD CONSTRAINT accounts_class_matches_type CHECK (
    (type IN ('checking','savings','cash','investment','retirement') AND class = 'asset')
    OR (type IN ('credit_card','loan') AND class = 'liability')
);

ALTER TABLE accounts DROP CONSTRAINT accounts_type_check;
ALTER TABLE accounts ADD CONSTRAINT accounts_type_check
    CHECK (type IN ('checking','savings','cash','investment','retirement','credit_card','loan'));
