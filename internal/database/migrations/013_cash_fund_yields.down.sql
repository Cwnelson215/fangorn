DROP TABLE IF EXISTS security_yields;
ALTER TABLE accounts DROP CONSTRAINT accounts_cash_fund_holds_securities;
ALTER TABLE accounts DROP CONSTRAINT accounts_cash_fund_pair;
ALTER TABLE accounts DROP COLUMN cash_fund_since;
ALTER TABLE accounts DROP COLUMN cash_fund;
