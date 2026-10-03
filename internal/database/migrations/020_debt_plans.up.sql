-- A planned monthly paydown on a credit card or loan, versioned by month like
-- goal_plans: an amount applies from its month until the next row, and a NULL
-- amount stops it. Payments themselves are ordinary transfers into the account;
-- nothing about them is stored here.

CREATE TABLE debt_plans (
    id             SERIAL PRIMARY KEY,
    household_id   INTEGER NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    account_id     INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    effective_from DATE NOT NULL,
    amount         NUMERIC(14,2),

    CONSTRAINT debt_plans_month CHECK (effective_from = date_trunc('month', effective_from)::date),
    CONSTRAINT debt_plans_positive CHECK (amount IS NULL OR amount > 0),
    CONSTRAINT debt_plans_unique UNIQUE (account_id, effective_from)
);
