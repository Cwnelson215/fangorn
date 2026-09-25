-- A category can name the account its spending goes on: "gas always goes on
-- the Visa". A receipt filed under the category posts there whatever card it
-- shows, and picking the category on /add switches to that account.
--
-- The iPhone Shortcut is the exception: it logs to its own key's account, and a
-- receipt it uploads is matched the ordinary way. receipts.via_shortcut records
-- which uploads came from it, since reading happens later, away from the request.

ALTER TABLE categories ADD COLUMN default_account_id INTEGER REFERENCES accounts(id) ON DELETE SET NULL;
ALTER TABLE receipts ADD COLUMN via_shortcut BOOLEAN NOT NULL DEFAULT false;
