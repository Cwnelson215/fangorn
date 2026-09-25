-- An investment account's cash yield, looked up rather than typed in.
--
-- Uninvested cash at a brokerage sits in a money market fund (SPAXX at
-- Fidelity) whose yield moves every day. An account names that fund in
-- cash_fund, and the scheduler records the fund's published yield once a day
-- in security_yields. Like securities and security_prices this is shared market
-- data, not scoped to a household: two accounts holding SPAXX read one history.
--
-- The fund only governs from cash_fund_since, the day it was linked, so linking
-- never back-posts months that were entered by hand; manual rates in
-- savings_rates still cover the months before it.

ALTER TABLE accounts ADD COLUMN cash_fund TEXT REFERENCES securities(symbol);
ALTER TABLE accounts ADD COLUMN cash_fund_since DATE;
ALTER TABLE accounts ADD CONSTRAINT accounts_cash_fund_pair
    CHECK ((cash_fund IS NULL) = (cash_fund_since IS NULL));
ALTER TABLE accounts ADD CONSTRAINT accounts_cash_fund_holds_securities
    CHECK (cash_fund IS NULL OR type IN ('investment','retirement'));

CREATE TABLE security_yields (
    symbol      TEXT NOT NULL REFERENCES securities(symbol) ON DELETE CASCADE,
    as_of       DATE NOT NULL,
    -- Percent, as published: 3.330 means a 3.33% yield. For a money market fund
    -- this is the 7-day yield, a simple annual rate.
    yield       NUMERIC(6,3) NOT NULL,
    fetched_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (symbol, as_of),
    CONSTRAINT security_yields_range CHECK (yield >= 0 AND yield < 100)
);
