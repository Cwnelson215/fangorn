-- The monthly plan: income lands in one account, and savings goals take a
-- share of it each month.
--
-- households.income_account_id is where income is logged by default — the
-- account money is then distributed from.
--
-- A goal's target is now how much to ADD to its account, not the balance it
-- should reach: progress is the money moved into the account since started_on
-- (transfers in less transfers out). Existing goals start from the day they were
-- created. monthly_amount is the goal's line in the monthly budget, taken out of
-- expected income alongside budgeted spending.

ALTER TABLE households ADD COLUMN income_account_id INTEGER REFERENCES accounts(id) ON DELETE SET NULL;

ALTER TABLE goals ADD COLUMN started_on DATE;
UPDATE goals SET started_on = created_at::date;
ALTER TABLE goals ALTER COLUMN started_on SET NOT NULL;

ALTER TABLE goals ADD COLUMN monthly_amount NUMERIC(14,2);
ALTER TABLE goals ADD CONSTRAINT goals_monthly_positive CHECK (monthly_amount IS NULL OR monthly_amount > 0);
