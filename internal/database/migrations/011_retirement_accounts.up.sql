-- Retirement accounts: an investment account with rules on top.
--
-- A Roth IRA or a 401(k) holds securities exactly like a brokerage account —
-- trades, holdings and prices all apply — but the money isn't spendable, and
-- how it is taxed depends on the plan. It gets its own type so it groups, labels
-- and totals separately, and so rules like contribution limits have somewhere to
-- live. Code decides "can this account hold securities" with
-- models.HoldsSecurities, never by comparing against 'investment' alone.

ALTER TABLE accounts DROP CONSTRAINT accounts_type_check;
ALTER TABLE accounts ADD CONSTRAINT accounts_type_check
    CHECK (type IN ('checking','savings','cash','investment','retirement','credit_card','loan'));

ALTER TABLE accounts DROP CONSTRAINT accounts_class_matches_type;
ALTER TABLE accounts ADD CONSTRAINT accounts_class_matches_type CHECK (
    (type IN ('checking','savings','cash','investment','retirement') AND class = 'asset')
    OR (type IN ('credit_card','loan') AND class = 'liability')
);

-- Roth money went in after tax and comes out tax-free; traditional money went in
-- pre-tax and is taxed when withdrawn. Every retirement account is one or the
-- other, and nothing else carries either.
ALTER TABLE accounts ADD COLUMN tax_treatment TEXT;
ALTER TABLE accounts ADD CONSTRAINT accounts_tax_treatment_check
    CHECK (tax_treatment IN ('roth','traditional'));
ALTER TABLE accounts ADD CONSTRAINT accounts_tax_treatment_on_retirement
    CHECK ((type = 'retirement') = (tax_treatment IS NOT NULL));
