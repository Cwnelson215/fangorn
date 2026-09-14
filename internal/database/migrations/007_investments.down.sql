-- Reverse 007. Trade cash legs have no meaning without their trades, so they are
-- removed before the old kind constraint (which does not allow 'trade') returns.

DELETE FROM transactions WHERE kind = 'trade';

DROP INDEX IF EXISTS idx_transactions_trade;
ALTER TABLE transactions DROP CONSTRAINT IF EXISTS transactions_trade_has_trade;
ALTER TABLE transactions DROP CONSTRAINT IF EXISTS transactions_trade_fk;

ALTER TABLE transactions DROP CONSTRAINT transactions_sign_matches_kind;
ALTER TABLE transactions ADD CONSTRAINT transactions_sign_matches_kind CHECK (
    (kind = 'income'  AND amount > 0)
    OR (kind = 'expense' AND amount < 0)
    OR kind = 'transfer'
);

ALTER TABLE transactions DROP CONSTRAINT transactions_kind_check;
ALTER TABLE transactions ADD CONSTRAINT transactions_kind_check
    CHECK (kind IN ('income','expense','transfer'));

ALTER TABLE transactions DROP COLUMN IF EXISTS trade_id;

DROP TABLE IF EXISTS trades;
DROP TABLE IF EXISTS security_prices;
DROP TABLE IF EXISTS securities;
