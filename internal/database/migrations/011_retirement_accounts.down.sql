-- Retirement accounts become plain investment accounts again; their trades and
-- holdings are untouched.
ALTER TABLE accounts DROP CONSTRAINT accounts_tax_treatment_on_retirement;
ALTER TABLE accounts DROP CONSTRAINT accounts_tax_treatment_check;
UPDATE accounts SET type = 'investment' WHERE type = 'retirement';
ALTER TABLE accounts DROP COLUMN tax_treatment;

ALTER TABLE accounts DROP CONSTRAINT accounts_class_matches_type;
ALTER TABLE accounts ADD CONSTRAINT accounts_class_matches_type CHECK (
    (type IN ('checking','savings','cash','investment') AND class = 'asset')
    OR (type IN ('credit_card','loan') AND class = 'liability')
);

ALTER TABLE accounts DROP CONSTRAINT accounts_type_check;
ALTER TABLE accounts ADD CONSTRAINT accounts_type_check
    CHECK (type IN ('checking','savings','cash','investment','credit_card','loan'));
