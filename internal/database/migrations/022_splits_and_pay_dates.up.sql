-- Stock splits, found and confirmed the same way dividends are, and the day a
-- dividend is actually paid.

-- The pay date of a declared dividend, when the provider publishes one (it
-- does for stocks, not for mutual funds). NULL = not known.
ALTER TABLE security_dividends ADD COLUMN pay_date DATE;

-- Shared market data: every `denominator` shares became `numerator` on the
-- split date. 10 and 1 is a ten-for-one split; 1 and 10 a reverse split.
CREATE TABLE security_splits (
    symbol       TEXT NOT NULL REFERENCES securities(symbol) ON DELETE CASCADE,
    split_date   DATE NOT NULL,
    numerator    NUMERIC(12,4) NOT NULL,
    denominator  NUMERIC(12,4) NOT NULL,

    PRIMARY KEY (symbol, split_date),
    CONSTRAINT security_splits_ratio CHECK (numerator > 0 AND denominator > 0 AND numerator <> denominator)
);

-- One row per account, symbol and split date. Confirming rewrites the
-- account's trades from before the split into post-split shares and prices —
-- the terms the price history is in once the provider adjusts it — so nothing
-- is posted and there is no row to link to.
CREATE TABLE splits (
    id            SERIAL PRIMARY KEY,
    household_id  INTEGER NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    account_id    INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    symbol        TEXT NOT NULL REFERENCES securities(symbol),
    split_date    DATE NOT NULL,
    numerator     NUMERIC(12,4) NOT NULL,
    denominator   NUMERIC(12,4) NOT NULL,
    -- shares held going into the split, as the trades said when it was found
    shares        NUMERIC(20,8) NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at   TIMESTAMPTZ,

    CONSTRAINT splits_status_check CHECK (status IN ('pending','confirmed','dismissed')),
    CONSTRAINT splits_once UNIQUE (account_id, symbol, split_date)
);

CREATE INDEX splits_pending ON splits (household_id) WHERE status = 'pending';
