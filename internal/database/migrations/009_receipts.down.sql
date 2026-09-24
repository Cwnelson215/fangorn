-- Reverse 009. A transaction with source 'receipt' cannot exist under the old
-- constraint, and relabelling it 'manual' would claim someone typed it, so the
-- rows go with their receipts — the same call 007 and 008 make.

DROP TABLE receipts;

DELETE FROM transactions WHERE source = 'receipt';

ALTER TABLE transactions DROP CONSTRAINT transactions_source_check;
ALTER TABLE transactions ADD CONSTRAINT transactions_source_check
    CHECK (source IN ('manual','recurring'));
