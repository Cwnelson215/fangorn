-- Goals in one order. Several goals can share an account, so money added to it
-- fills them in priority order (1 first) and money leaving it comes off the
-- lowest priority first — see internal/goalfill. Existing goals keep the order
-- the goals list showed them in; monthly goals follow.
ALTER TABLE goals ADD COLUMN priority INTEGER;

UPDATE goals g SET priority = o.n
FROM (
  SELECT id, ROW_NUMBER() OVER (
    PARTITION BY household_id
    ORDER BY month IS NOT NULL, month, achieved_at IS NOT NULL, target_date NULLS LAST, name, id
  ) AS n
  FROM goals
) o
WHERE o.id = g.id;

ALTER TABLE goals ALTER COLUMN priority SET NOT NULL;
