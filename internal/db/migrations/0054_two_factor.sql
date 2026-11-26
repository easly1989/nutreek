-- +goose Up
-- The second sign-in step (#335). One row per account that has set anything up.
-- totp_secret is sealed with NUTREEK_SECRET_KEY; totp_on is 0 while the person is still
-- setting it up. last_step is the last 30-second step accepted, so a code is used once.
-- The email code is kept as a salted hash with its expiry. failed and locked_until count
-- wrong codes: five in a row lock the step for five minutes. Times are Unix seconds.
CREATE TABLE user_2fa (
    user_id            INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    totp_secret        TEXT    NOT NULL DEFAULT '',
    totp_on            INTEGER NOT NULL DEFAULT 0 CHECK (totp_on IN (0, 1)),
    last_step          INTEGER NOT NULL DEFAULT 0,
    email_on           INTEGER NOT NULL DEFAULT 0 CHECK (email_on IN (0, 1)),
    email_code_salt    TEXT    NOT NULL DEFAULT '',
    email_code_hash    TEXT    NOT NULL DEFAULT '',
    email_code_expires INTEGER NOT NULL DEFAULT 0,
    email_code_sent    INTEGER NOT NULL DEFAULT 0,
    failed             INTEGER NOT NULL DEFAULT 0,
    locked_until       INTEGER NOT NULL DEFAULT 0
);

-- One-time recovery codes: only a salted hash is kept, and used_at says when one was used.
CREATE TABLE recovery_codes (
    id      INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    salt    TEXT    NOT NULL,
    hash    TEXT    NOT NULL,
    used_at INTEGER
);
CREATE INDEX idx_recovery_codes_user ON recovery_codes (user_id);

-- +goose Down
DROP TABLE recovery_codes;
DROP TABLE user_2fa;
