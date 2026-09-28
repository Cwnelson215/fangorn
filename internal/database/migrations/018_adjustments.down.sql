DELETE FROM transactions WHERE kind = 'adjustment';

ALTER TABLE transactions DROP CONSTRAINT transactions_adjustment_uncategorized;

ALTER TABLE transactions DROP CONSTRAINT transactions_sign_matches_kind;
ALTER TABLE transactions ADD CONSTRAINT transactions_sign_matches_kind CHECK (
    (kind = 'income'  AND amount > 0)
    OR (kind = 'expense' AND amount < 0)
    OR (kind = 'refund'  AND amount > 0)
    OR kind IN ('transfer','trade')
);

ALTER TABLE transactions DROP CONSTRAINT transactions_kind_check;
ALTER TABLE transactions ADD CONSTRAINT transactions_kind_check
    CHECK (kind IN ('income','expense','transfer','trade','refund'));
