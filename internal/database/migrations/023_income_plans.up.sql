-- Expected income is one monthly amount for the household, not a budget per
-- income category: every income transaction counts toward it, whatever its
-- category. Versioned by month like debt_plans — an amount applies from its
-- month until the next row, and a NULL amount stops it.

CREATE TABLE income_plans (
    id             SERIAL PRIMARY KEY,
    household_id   INTEGER NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    effective_from DATE NOT NULL,
    amount         NUMERIC(14,2),

    CONSTRAINT income_plans_month CHECK (effective_from = date_trunc('month', effective_from)::date),
    CONSTRAINT income_plans_positive CHECK (amount IS NULL OR amount > 0),
    CONSTRAINT income_plans_unique UNIQUE (household_id, effective_from)
);

-- Every month an income budget changed becomes a plan row holding the sum of
-- the income budgets in force that month, so each month expects what it did.
INSERT INTO income_plans (household_id, effective_from, amount)
SELECT m.household_id, m.effective_from, NULLIF((
         SELECT COALESCE(SUM(x.amount), 0) FROM (
           SELECT DISTINCT ON (b.category_id) b.amount, b.archived_at
           FROM budgets b
           JOIN categories c ON c.id = b.category_id AND c.kind = 'income' AND c.archived_at IS NULL
           WHERE b.household_id = m.household_id AND b.effective_from <= m.effective_from
           ORDER BY b.category_id, b.effective_from DESC
         ) x
         WHERE x.archived_at IS NULL), 0)
FROM (
  SELECT DISTINCT b.household_id, b.effective_from
  FROM budgets b JOIN categories c ON c.id = b.category_id AND c.kind = 'income'
) m;

DELETE FROM budgets b USING categories c
WHERE c.id = b.category_id AND c.kind = 'income';
