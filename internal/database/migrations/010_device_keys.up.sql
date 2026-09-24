-- Device keys: what an iPhone Shortcut sends instead of a login cookie.
--
-- A Shortcut runs outside the browser, so it can't carry the session cookie.
-- Each phone gets its own key instead, made and revoked from the app. A key is
-- deliberately narrow: it can list category names, log an income, expense or
-- refund to the account it was made for, and send a receipt photo — nothing
-- else, and it can't read anything back. It is shown once when created; only
-- its SHA-256 is stored, so a database dump can't be replayed as a key.

CREATE TABLE device_keys (
    id            SERIAL PRIMARY KEY,
    household_id  INTEGER NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    name          TEXT NOT NULL CHECK (length(btrim(name)) > 0),
    -- Where the Shortcut's entries land. Deleting the account retires its keys.
    account_id    INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    token_hash    TEXT NOT NULL UNIQUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at  TIMESTAMPTZ
);

CREATE INDEX device_keys_household ON device_keys (household_id);
