-- Two kinds of savings goals.
--
-- A monthly goal belongs to one month (goals.month, the first of it): its
-- target is what to put away that month, it counts only money added inside the
-- month, and it closes with the month. A long-term goal has no month; its
-- monthly share is a plan versioned by month in goal_plans, like budgets'
-- effective_from: an amount applies from its month until the next row, and a
-- NULL amount stops it. Changing the plan from December leaves October and
-- November as they were.

ALTER TABLE goals ADD COLUMN month DATE;
ALTER TABLE goals ADD CONSTRAINT goals_month_first
    CHECK (month IS NULL OR month = date_trunc('month', month)::date);

CREATE TABLE goal_plans (
    id             SERIAL PRIMARY KEY,
    goal_id        INTEGER NOT NULL REFERENCES goals(id) ON DELETE CASCADE,
    effective_from DATE NOT NULL,
    amount         NUMERIC(14,2),

    CONSTRAINT goal_plans_month CHECK (effective_from = date_trunc('month', effective_from)::date),
    CONSTRAINT goal_plans_positive CHECK (amount IS NULL OR amount > 0),
    CONSTRAINT goal_plans_unique UNIQUE (goal_id, effective_from)
);

-- Every monthly amount so far was set within a day of 015 shipping, while
-- planning the month ahead. A goal whose whole target is one month's amount is a
-- one-month goal for the month after it was created; any other becomes a
-- long-term plan starting that month.
UPDATE goals SET month = (date_trunc('month', started_on) + INTERVAL '1 month')::date
 WHERE monthly_amount IS NOT NULL AND monthly_amount = target_amount;

INSERT INTO goal_plans (goal_id, effective_from, amount)
SELECT id, (date_trunc('month', started_on) + INTERVAL '1 month')::date, monthly_amount
  FROM goals WHERE monthly_amount IS NOT NULL AND month IS NULL;

ALTER TABLE goals DROP CONSTRAINT goals_monthly_positive;
ALTER TABLE goals DROP COLUMN monthly_amount;
