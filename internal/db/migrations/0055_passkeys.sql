-- +goose Up
-- Passkeys (#336): WebAuthn credentials that sign a person in with no password. credential
-- is the library's JSON for the credential (public key, sign count, flags); the account
-- finds it by credential_id. user_handle is the opaque random id the person has at the
-- authenticator, the same on every row of an account. Times are Unix seconds.
CREATE TABLE passkeys (
    id            INTEGER PRIMARY KEY,
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_handle   BLOB    NOT NULL,
    credential_id BLOB    NOT NULL UNIQUE,
    name          TEXT    NOT NULL,
    credential    TEXT    NOT NULL,
    created_at    INTEGER NOT NULL,
    last_used_at  INTEGER
);
CREATE INDEX idx_passkeys_user ON passkeys (user_id);

-- +goose Down
DROP TABLE passkeys;
