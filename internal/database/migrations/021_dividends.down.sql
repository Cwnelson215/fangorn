UPDATE transactions SET source = 'manual' WHERE source = 'dividend';
ALTER TABLE transactions DROP CONSTRAINT transactions_source_check;
ALTER TABLE transactions ADD CONSTRAINT transactions_source_check
    CHECK (source IN ('manual','recurring','receipt','interest'));

DROP TABLE dividends;
DROP TABLE security_dividends;
ALTER TABLE securities DROP COLUMN dividends_checked_at;
