-- A balance adjustment: money taken off (or put back on) an account's books
-- without it being earned, spent or moved — "drop the remaining balance" when
-- closing an account whose money went nowhere the ledger tracks.
--
-- Every income and spending total lists the kinds it counts, so an adjustment
-- stays out of all of them; it changes the balance, and so net worth, only. It
-- has no category because it isn't spending of any kind.
ALTER TABLE transactions DROP CONSTRAINT transactions_kind_check;
ALTER TABLE transactions ADD CONSTRAINT transactions_kind_check
    CHECK (kind IN ('income','expense','transfer','trade','refund','adjustment'));

ALTER TABLE transactions DROP CONSTRAINT transactions_sign_matches_kind;
ALTER TABLE transactions ADD CONSTRAINT transactions_sign_matches_kind CHECK (
    (kind = 'income'  AND amount > 0)
    OR (kind = 'expense' AND amount < 0)
    OR (kind = 'refund'  AND amount > 0)
    OR (kind = 'adjustment' AND amount <> 0)
    OR kind IN ('transfer','trade')
);

ALTER TABLE transactions ADD CONSTRAINT transactions_adjustment_uncategorized
    CHECK (kind <> 'adjustment' OR category_id IS NULL);
