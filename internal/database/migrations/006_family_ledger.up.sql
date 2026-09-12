-- Pivot from bank-sync to a hand-kept family ledger.
--
-- Everything the Teller/CSV/Gmail era created is dropped: there is no data worth
-- preserving and the shapes no longer fit. See _deprecated/README.md.
--
-- Sign convention, applied everywhere from here on:
--   transactions.amount is signed RELATIVE TO THE ACCOUNT.
--   positive = money in, negative = money out.
--   Liability accounts (credit_card, loan) therefore carry NEGATIVE balances, which
--   makes both of these correct without special-casing account type:
--     balance   = starting_balance + SUM(amount)
--     net worth = SUM(balance)

DROP TABLE IF EXISTS transfers CASCADE;
DROP TABLE IF EXISTS csv_imports CASCADE;
DROP TABLE IF EXISTS bank_csv_formats CASCADE;
DROP TABLE IF EXISTS gmail_watch_state CASCADE;
DROP TABLE IF EXISTS transactions CASCADE;
DROP TABLE IF EXISTS accounts CASCADE;
DROP TABLE IF EXISTS linked_institutions CASCADE;
DROP TABLE IF EXISTS net_worth_snapshots CASCADE;

-- ---------------------------------------------------------------------------
-- households
--
-- Tenancy exists from day one even though there is no users table yet (auth is
-- still the single shared APP_PASSWORD). Every app table carries household_id so
-- that adding real users later is an additive migration, not a rewrite.
-- ---------------------------------------------------------------------------

CREATE TABLE households (
    id          SERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    timezone    TEXT NOT NULL DEFAULT 'America/Denver',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO households (id, name) VALUES (1, 'Our Household');
SELECT setval('households_id_seq', 1, true);

-- ---------------------------------------------------------------------------
-- accounts
-- ---------------------------------------------------------------------------

CREATE TABLE accounts (
    id                     SERIAL PRIMARY KEY,
    household_id           INTEGER NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    name                   TEXT NOT NULL,
    institution_name       TEXT,
    type                   TEXT NOT NULL,
    class                  TEXT NOT NULL,
    mask                   TEXT,
    starting_balance       NUMERIC(14,2) NOT NULL DEFAULT 0,
    starting_balance_date  DATE NOT NULL DEFAULT CURRENT_DATE,
    currency               TEXT NOT NULL DEFAULT 'USD',
    color                  TEXT,
    notes                  TEXT,
    archived_at            TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT accounts_type_check
        CHECK (type IN ('checking','savings','cash','investment','credit_card','loan')),
    CONSTRAINT accounts_class_check
        CHECK (class IN ('asset','liability')),
    -- class is fully determined by type; the CHECK keeps them from drifting apart
    CONSTRAINT accounts_class_matches_type CHECK (
        (type IN ('checking','savings','cash','investment') AND class = 'asset')
        OR (type IN ('credit_card','loan') AND class = 'liability')
    )
);

CREATE INDEX idx_accounts_household ON accounts(household_id) WHERE archived_at IS NULL;

-- ---------------------------------------------------------------------------
-- categories
-- ---------------------------------------------------------------------------

CREATE TABLE categories (
    id            SERIAL PRIMARY KEY,
    household_id  INTEGER NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    kind          TEXT NOT NULL,
    color         TEXT,
    parent_id     INTEGER REFERENCES categories(id) ON DELETE SET NULL,
    archived_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT categories_kind_check CHECK (kind IN ('income','expense'))
);

CREATE UNIQUE INDEX idx_categories_household_name ON categories(household_id, lower(name));

INSERT INTO categories (household_id, name, kind, color) VALUES
    (1, 'Groceries',       'expense', '#4ecca3'),
    (1, 'Dining Out',      'expense', '#ff6b6b'),
    (1, 'Housing',         'expense', '#4ecdc4'),
    (1, 'Utilities',       'expense', '#45b7d1'),
    (1, 'Transportation',  'expense', '#96ceb4'),
    (1, 'Insurance',       'expense', '#ffeaa7'),
    (1, 'Health',          'expense', '#fd79a8'),
    (1, 'Subscriptions',   'expense', '#a29bfe'),
    (1, 'Entertainment',   'expense', '#55a3f0'),
    (1, 'Shopping',        'expense', '#dfe6e9'),
    (1, 'Kids',            'expense', '#ffb8b8'),
    (1, 'Debt Payment',    'expense', '#e17055'),
    (1, 'Other',           'expense', '#b2bec3'),
    (1, 'Paycheck',        'income',  '#4ecca3'),
    (1, 'Bonus',           'income',  '#00b894'),
    (1, 'Interest',        'income',  '#00cec9'),
    (1, 'Reimbursement',   'income',  '#81ecec'),
    (1, 'Other Income',    'income',  '#b2bec3');

-- ---------------------------------------------------------------------------
-- recurring_rules
--
-- One table covers both subscription charges and internal transfers. A transfer
-- rule sets to_account_id; income/expense rules leave it NULL.
-- ---------------------------------------------------------------------------

CREATE TABLE recurring_rules (
    id                   SERIAL PRIMARY KEY,
    household_id         INTEGER NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    name                 TEXT NOT NULL,
    vendor               TEXT,
    kind                 TEXT NOT NULL,
    account_id           INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    to_account_id        INTEGER REFERENCES accounts(id) ON DELETE CASCADE,
    category_id          INTEGER REFERENCES categories(id) ON DELETE SET NULL,
    amount               NUMERIC(14,2) NOT NULL,
    frequency            TEXT NOT NULL,
    interval_count       INTEGER NOT NULL DEFAULT 1,
    day_of_month         INTEGER,
    second_day_of_month  INTEGER,
    day_of_week          INTEGER,
    month_of_year        INTEGER,
    start_date           DATE NOT NULL,
    end_date             DATE,
    next_due_date        DATE,
    auto_post            BOOLEAN NOT NULL DEFAULT TRUE,
    reminder_lead_days   INTEGER,
    paused_at            TIMESTAMPTZ,
    notes                TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT recurring_kind_check CHECK (kind IN ('income','expense','transfer')),
    CONSTRAINT recurring_frequency_check CHECK (frequency IN
        ('daily','weekly','biweekly','semimonthly','monthly','quarterly','yearly')),
    CONSTRAINT recurring_interval_check CHECK (interval_count >= 1),
    CONSTRAINT recurring_amount_positive CHECK (amount > 0),
    -- transfers need a destination and it must differ from the source;
    -- income/expense must not have one
    CONSTRAINT recurring_transfer_shape CHECK (
        (kind = 'transfer' AND to_account_id IS NOT NULL AND to_account_id <> account_id)
        OR (kind <> 'transfer' AND to_account_id IS NULL)
    ),
    CONSTRAINT recurring_day_of_month_check
        CHECK (day_of_month IS NULL OR day_of_month BETWEEN 1 AND 31),
    CONSTRAINT recurring_second_day_check
        CHECK (second_day_of_month IS NULL OR second_day_of_month BETWEEN 1 AND 31),
    CONSTRAINT recurring_day_of_week_check
        CHECK (day_of_week IS NULL OR day_of_week BETWEEN 0 AND 6),
    CONSTRAINT recurring_month_check
        CHECK (month_of_year IS NULL OR month_of_year BETWEEN 1 AND 12),
    CONSTRAINT recurring_end_after_start
        CHECK (end_date IS NULL OR end_date >= start_date)
);

CREATE INDEX idx_recurring_household ON recurring_rules(household_id);
CREATE INDEX idx_recurring_due ON recurring_rules(next_due_date) WHERE paused_at IS NULL;

-- ---------------------------------------------------------------------------
-- transactions
-- ---------------------------------------------------------------------------

CREATE TABLE transactions (
    id                 SERIAL PRIMARY KEY,
    household_id       INTEGER NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    account_id         INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    date               DATE NOT NULL,
    amount             NUMERIC(14,2) NOT NULL,
    kind               TEXT NOT NULL,
    description        TEXT NOT NULL,
    merchant           TEXT,
    category_id        INTEGER REFERENCES categories(id) ON DELETE SET NULL,
    notes              TEXT,
    transfer_group_id  UUID,
    recurring_rule_id  INTEGER REFERENCES recurring_rules(id) ON DELETE SET NULL,
    source             TEXT NOT NULL DEFAULT 'manual',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT transactions_kind_check CHECK (kind IN ('income','expense','transfer')),
    CONSTRAINT transactions_source_check CHECK (source IN ('manual','recurring')),
    CONSTRAINT transactions_amount_nonzero CHECK (amount <> 0),
    -- income is always money in, expense always money out; transfers go either way
    CONSTRAINT transactions_sign_matches_kind CHECK (
        (kind = 'income'  AND amount > 0)
        OR (kind = 'expense' AND amount < 0)
        OR kind = 'transfer'
    ),
    CONSTRAINT transactions_transfer_has_group CHECK (
        kind <> 'transfer' OR transfer_group_id IS NOT NULL
    )
);

CREATE INDEX idx_transactions_household_date ON transactions(household_id, date DESC);
CREATE INDEX idx_transactions_account_date ON transactions(account_id, date DESC);
CREATE INDEX idx_transactions_category ON transactions(category_id);
CREATE INDEX idx_transactions_transfer_group ON transactions(transfer_group_id)
    WHERE transfer_group_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- recurring_occurrences
--
-- One row per (rule, due date). The UNIQUE constraint is the whole idempotency
-- story: the scheduler can run any number of times, catch up after downtime, or
-- overlap with another process, and a charge still posts exactly once.
-- ---------------------------------------------------------------------------

CREATE TABLE recurring_occurrences (
    id              SERIAL PRIMARY KEY,
    rule_id         INTEGER NOT NULL REFERENCES recurring_rules(id) ON DELETE CASCADE,
    due_date        DATE NOT NULL,
    status          TEXT NOT NULL DEFAULT 'scheduled',
    transaction_id  INTEGER REFERENCES transactions(id) ON DELETE SET NULL,
    posted_at       TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT occurrence_status_check CHECK (status IN ('scheduled','posted','skipped')),
    CONSTRAINT occurrence_unique UNIQUE (rule_id, due_date)
);

CREATE INDEX idx_occurrences_due ON recurring_occurrences(due_date, status);

-- ---------------------------------------------------------------------------
-- budgets
--
-- effective_from lets a budget change over time without losing history: the
-- amount in force for a month is the newest row with effective_from <= that month.
-- ---------------------------------------------------------------------------

CREATE TABLE budgets (
    id              SERIAL PRIMARY KEY,
    household_id    INTEGER NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    category_id     INTEGER NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    period          TEXT NOT NULL DEFAULT 'monthly',
    amount          NUMERIC(14,2) NOT NULL,
    effective_from  DATE NOT NULL,
    archived_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT budgets_period_check CHECK (period IN ('monthly')),
    CONSTRAINT budgets_amount_positive CHECK (amount > 0),
    CONSTRAINT budgets_unique UNIQUE (household_id, category_id, effective_from)
);

CREATE INDEX idx_budgets_household ON budgets(household_id) WHERE archived_at IS NULL;

-- ---------------------------------------------------------------------------
-- goals
-- ---------------------------------------------------------------------------

CREATE TABLE goals (
    id             SERIAL PRIMARY KEY,
    household_id   INTEGER NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    target_amount  NUMERIC(14,2) NOT NULL,
    target_date    DATE,
    account_id     INTEGER REFERENCES accounts(id) ON DELETE SET NULL,
    notes          TEXT,
    achieved_at    TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT goals_target_positive CHECK (target_amount > 0)
);

CREATE INDEX idx_goals_household ON goals(household_id);

CREATE TABLE goal_contributions (
    id              SERIAL PRIMARY KEY,
    goal_id         INTEGER NOT NULL REFERENCES goals(id) ON DELETE CASCADE,
    date            DATE NOT NULL,
    amount          NUMERIC(14,2) NOT NULL,
    transaction_id  INTEGER REFERENCES transactions(id) ON DELETE SET NULL,
    note            TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_goal_contributions_goal ON goal_contributions(goal_id, date);

-- ---------------------------------------------------------------------------
-- net_worth_snapshots
-- ---------------------------------------------------------------------------

CREATE TABLE net_worth_snapshots (
    id                 SERIAL PRIMARY KEY,
    household_id       INTEGER NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    total_assets       NUMERIC(14,2) NOT NULL,
    total_liabilities  NUMERIC(14,2) NOT NULL,
    net_worth          NUMERIC(14,2) NOT NULL,
    snapshot_date      DATE NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT net_worth_unique UNIQUE (household_id, snapshot_date)
);
