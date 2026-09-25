ALTER TABLE goals ADD COLUMN monthly_amount NUMERIC(14,2);

UPDATE goals g SET monthly_amount = p.amount
  FROM (SELECT DISTINCT ON (goal_id) goal_id, amount FROM goal_plans
        ORDER BY goal_id, effective_from DESC) p
 WHERE p.goal_id = g.id;
UPDATE goals SET monthly_amount = target_amount WHERE month IS NOT NULL;

ALTER TABLE goals ADD CONSTRAINT goals_monthly_positive CHECK (monthly_amount IS NULL OR monthly_amount > 0);
DROP TABLE goal_plans;
ALTER TABLE goals DROP CONSTRAINT goals_month_first;
ALTER TABLE goals DROP COLUMN month;
