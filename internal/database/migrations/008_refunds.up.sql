-- A refund is money coming back from a category you already spent in: a return,
-- a reimbursement, a cancelled charge, a duplicate payment reversed.
--
-- It is its own kind rather than an income row because it has to *reduce that
-- category's spending*, not add to earnings. Taking $80 of groceries back to the
-- store leaves you $80 further under the grocery budget and no better off for
-- the month; booking it as income would claim both that you spent the full
-- amount and that you earned $80, and would flatter every income total on the
-- dashboard.
--
-- The sign rule is unchanged from every other kind: money in, so positive. What
-- makes a refund a refund is that everywhere expenses are summed counts it as a
-- negative, so `spent` nets out on its own.

ALTER TABLE transactions DROP CONSTRAINT transactions_kind_check;
ALTER TABLE transactions ADD CONSTRAINT transactions_kind_check
    CHECK (kind IN ('income','expense','transfer','trade','refund'));

ALTER TABLE transactions DROP CONSTRAINT transactions_sign_matches_kind;
ALTER TABLE transactions ADD CONSTRAINT transactions_sign_matches_kind CHECK (
    (kind = 'income'  AND amount > 0)
    OR (kind = 'expense' AND amount < 0)
    OR (kind = 'refund'  AND amount > 0)
    OR kind IN ('transfer','trade')
);

-- A refund that does not say what it came back from cannot offset anything, so
-- it would be a positive number sitting outside every spending total — exactly
-- the silent-disappearance case this migration exists to close.
--
-- categories.id is ON DELETE SET NULL, so this also means a category with
-- refunds against it cannot be deleted out from under them. That is already how
-- the app behaves: DeleteCategory archives a category that is in use rather than
-- deleting it.
ALTER TABLE transactions ADD CONSTRAINT transactions_refund_has_category
    CHECK (kind <> 'refund' OR category_id IS NOT NULL);
