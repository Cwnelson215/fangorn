-- Investment accounts: a trade log per account, priced from market data.
--
-- An investment account keeps its cash on the ordinary ledger (starting_balance
-- plus transactions) and adds holdings on top. Holdings are never stored as a
-- share count — they are replayed from the trade log, so a back-dated buy or a
-- corrected price fixes every figure derived from it.

-- ---------------------------------------------------------------------------
-- securities / security_prices
--
-- Deliberately NOT household-scoped. A fund's price is public market data, not
-- anything a household owns, and two families holding FZROX should share one
-- quote rather than each fetching their own. What a household holds lives in
-- trades, which is scoped like everything else.
-- ---------------------------------------------------------------------------

CREATE TABLE securities (
    symbol          TEXT PRIMARY KEY,
    name            TEXT,
    quote_type      TEXT,
    currency        TEXT NOT NULL DEFAULT 'USD',
    exchange        TEXT,
    -- Seeded from the first trade's price, so holdings have a value even when
    -- the price provider is unreachable or has never answered for this symbol.
    last_price      NUMERIC(18,6) NOT NULL,
    previous_close  NUMERIC(18,6),
    price_time      TIMESTAMPTZ,  -- NULL = seeded from a trade, never quoted
    fetched_at      TIMESTAMPTZ,  -- last successful fetch
    failed_at       TIMESTAMPTZ,  -- last failed fetch, for backoff
    fetch_error     TEXT,
    -- earliest date daily closes have been backfilled from; NULL = never
    history_from    DATE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT securities_symbol_upper CHECK (symbol <> '' AND symbol = UPPER(symbol))
);

CREATE TABLE security_prices (
    symbol      TEXT NOT NULL REFERENCES securities(symbol) ON DELETE CASCADE,
    price_date  DATE NOT NULL,
    close       NUMERIC(18,6) NOT NULL,

    PRIMARY KEY (symbol, price_date)
);

-- ---------------------------------------------------------------------------
-- trades
--
-- buy / sell       move cash: each writes one kind = 'trade' transaction on the
--                  same account, so cash stays starting_balance + SUM(amount).
-- reinvest         a dividend reinvested into more shares; no cash moves.
-- opening          a position already held when the account was set up,
--                  entered with its cost basis; no cash moves.
-- ---------------------------------------------------------------------------

CREATE TABLE trades (
    id            SERIAL PRIMARY KEY,
    household_id  INTEGER NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    account_id    INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    symbol        TEXT NOT NULL REFERENCES securities(symbol),
    side          TEXT NOT NULL,
    trade_date    DATE NOT NULL,
    shares        NUMERIC(20,8) NOT NULL,
    price         NUMERIC(18,6) NOT NULL,
    fees          NUMERIC(14,2) NOT NULL DEFAULT 0,
    -- The dollar figure: paid including fees (buy), received after fees (sell),
    -- or cost basis (reinvest, opening). Stored rather than derived because a
    -- fund order placed as "$500 of FZROX" is not exactly shares x price.
    amount        NUMERIC(14,2) NOT NULL,
    notes         TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT trades_side_check CHECK (side IN ('buy','sell','reinvest','opening')),
    CONSTRAINT trades_shares_positive CHECK (shares > 0),
    CONSTRAINT trades_price_nonnegative CHECK (price >= 0),
    CONSTRAINT trades_fees_nonnegative CHECK (fees >= 0),
    CONSTRAINT trades_amount_check CHECK (
        (side IN ('buy','sell') AND amount > 0)
        OR (side IN ('reinvest','opening') AND amount >= 0)
    ),
    -- target of the composite foreign key on transactions below
    CONSTRAINT trades_id_account UNIQUE (id, account_id)
);

CREATE INDEX idx_trades_account ON trades(account_id, trade_date, id);
CREATE INDEX idx_trades_household ON trades(household_id);
CREATE INDEX idx_trades_symbol ON trades(symbol);

-- ---------------------------------------------------------------------------
-- transactions: the cash leg of a buy or sell
--
-- trade_id cascades, so deleting a trade takes its cash leg with it. The key is
-- composite so the leg can only ever sit on its trade's own account. The leg
-- can be either sign (a buy is money out, a sell money in), like a transfer.
-- ---------------------------------------------------------------------------

ALTER TABLE transactions ADD COLUMN trade_id INTEGER;
ALTER TABLE transactions ADD CONSTRAINT transactions_trade_fk
    FOREIGN KEY (trade_id, account_id) REFERENCES trades(id, account_id) ON DELETE CASCADE;

ALTER TABLE transactions DROP CONSTRAINT transactions_kind_check;
ALTER TABLE transactions ADD CONSTRAINT transactions_kind_check
    CHECK (kind IN ('income','expense','transfer','trade'));

ALTER TABLE transactions DROP CONSTRAINT transactions_sign_matches_kind;
ALTER TABLE transactions ADD CONSTRAINT transactions_sign_matches_kind CHECK (
    (kind = 'income'  AND amount > 0)
    OR (kind = 'expense' AND amount < 0)
    OR kind IN ('transfer','trade')
);

ALTER TABLE transactions ADD CONSTRAINT transactions_trade_has_trade
    CHECK ((kind = 'trade') = (trade_id IS NOT NULL));

CREATE UNIQUE INDEX idx_transactions_trade ON transactions(trade_id)
    WHERE trade_id IS NOT NULL;
