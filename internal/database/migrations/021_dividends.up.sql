-- Dividends from held stocks and funds, found for the household and confirmed
-- with one tap rather than typed in.
--
-- The price provider publishes each dividend's ex-date and amount per share.
-- That is enough to know one is owed — shares held going into the ex-date ×
-- the amount — but not when it lands (the pay date is days or weeks later) or,
-- for a reinvested fund dividend, how many shares it bought. So nothing posts
-- by itself: each one waits as `pending` until someone confirms it.

-- When a symbol's dividends were last looked up; NULL = never.
ALTER TABLE securities ADD COLUMN dividends_checked_at TIMESTAMPTZ;

-- Shared market data, like security_prices: one row per declared dividend.
CREATE TABLE security_dividends (
    symbol   TEXT NOT NULL REFERENCES securities(symbol) ON DELETE CASCADE,
    ex_date  DATE NOT NULL,
    amount   NUMERIC(18,6) NOT NULL,

    PRIMARY KEY (symbol, ex_date),
    CONSTRAINT security_dividends_positive CHECK (amount > 0)
);

-- One row per account, symbol and ex-date, which is what makes finding them
-- idempotent. The row outlives the transaction or trade it posted (SET NULL),
-- so deleting a confirmed dividend doesn't bring the prompt back.
CREATE TABLE dividends (
    id             SERIAL PRIMARY KEY,
    household_id   INTEGER NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    account_id     INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    symbol         TEXT NOT NULL REFERENCES securities(symbol),
    ex_date        DATE NOT NULL,
    per_share      NUMERIC(18,6) NOT NULL,
    -- shares held going into the ex-date, and the estimate they make
    shares         NUMERIC(20,8) NOT NULL,
    amount         NUMERIC(14,2) NOT NULL,
    status         TEXT NOT NULL DEFAULT 'pending',
    transaction_id INTEGER REFERENCES transactions(id) ON DELETE SET NULL,
    trade_id       INTEGER REFERENCES trades(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at    TIMESTAMPTZ,

    CONSTRAINT dividends_status_check CHECK (status IN ('pending','confirmed','dismissed')),
    CONSTRAINT dividends_once UNIQUE (account_id, symbol, ex_date)
);

CREATE INDEX dividends_pending ON dividends (household_id) WHERE status = 'pending';

ALTER TABLE transactions DROP CONSTRAINT transactions_source_check;
ALTER TABLE transactions ADD CONSTRAINT transactions_source_check
    CHECK (source IN ('manual','recurring','receipt','interest','dividend'));
