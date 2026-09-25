-- High-yield savings: a savings account that earns interest by itself.
--
-- The account keeps a history of rates (an APY from a date), and once a month
-- is over the scheduler posts that month's interest as an ordinary income
-- transaction — month-end balance × the month's rate, each rate weighted by the
-- days it was in effect (internal/interest). Posting is idempotent the same way
-- recurring rules are: interest_postings has one row per account and month, and
-- the row and the transaction are written in one database transaction.

ALTER TABLE accounts DROP CONSTRAINT accounts_type_check;
ALTER TABLE accounts ADD CONSTRAINT accounts_type_check
    CHECK (type IN ('checking','savings','high_yield_savings','cash','investment','retirement',
                    'credit_card','loan'));

ALTER TABLE accounts DROP CONSTRAINT accounts_class_matches_type;
ALTER TABLE accounts ADD CONSTRAINT accounts_class_matches_type CHECK (
    (type IN ('checking','savings','high_yield_savings','cash','investment','retirement')
        AND class = 'asset')
    OR (type IN ('credit_card','loan') AND class = 'liability')
);

CREATE TABLE savings_rates (
    id              SERIAL PRIMARY KEY,
    household_id    INTEGER NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    -- Percent, as banks quote it: 4.350 means 4.35% APY.
    apy             NUMERIC(6,3) NOT NULL,
    effective_from  DATE NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT savings_rates_apy_range CHECK (apy >= 0 AND apy < 100),
    CONSTRAINT savings_rates_one_per_day UNIQUE (account_id, effective_from)
);

CREATE INDEX savings_rates_household ON savings_rates (household_id);

-- One row per account and month, written whether or not the month earned
-- anything, so a month is worked out once. transaction_id goes NULL when the
-- interest transaction is deleted: the month stays handled and is not reposted.
CREATE TABLE interest_postings (
    id              SERIAL PRIMARY KEY,
    household_id    INTEGER NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    account_id      INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    month           DATE NOT NULL,
    amount          NUMERIC(14,2) NOT NULL,
    transaction_id  INTEGER REFERENCES transactions(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT interest_postings_first_of_month CHECK (EXTRACT(DAY FROM month) = 1),
    CONSTRAINT interest_postings_once UNIQUE (account_id, month)
);

ALTER TABLE transactions DROP CONSTRAINT transactions_source_check;
ALTER TABLE transactions ADD CONSTRAINT transactions_source_check
    CHECK (source IN ('manual','recurring','receipt','interest'));
