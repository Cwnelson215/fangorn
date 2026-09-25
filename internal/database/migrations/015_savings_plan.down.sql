ALTER TABLE goals DROP CONSTRAINT goals_monthly_positive;
ALTER TABLE goals DROP COLUMN monthly_amount;
ALTER TABLE goals DROP COLUMN started_on;
ALTER TABLE households DROP COLUMN income_account_id;
