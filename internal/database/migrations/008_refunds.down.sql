-- Reverse 008. A refund cannot be represented under the old constraints, and
-- quietly turning one into income would overstate earnings and re-inflate the
-- spending it was cancelling, so the rows go — the same call 007 makes for trade
-- cash legs.

DELETE FROM transactions WHERE kind = 'refund';

ALTER TABLE transactions DROP CONSTRAINT IF EXISTS transactions_refund_has_category;

ALTER TABLE transactions DROP CONSTRAINT transactions_sign_matches_kind;
ALTER TABLE transactions ADD CONSTRAINT transactions_sign_matches_kind CHECK (
    (kind = 'income'  AND amount > 0)
    OR (kind = 'expense' AND amount < 0)
    OR kind IN ('transfer','trade')
);

ALTER TABLE transactions DROP CONSTRAINT transactions_kind_check;
ALTER TABLE transactions ADD CONSTRAINT transactions_kind_check
    CHECK (kind IN ('income','expense','transfer','trade'));
