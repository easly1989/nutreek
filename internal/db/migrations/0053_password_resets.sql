-- +goose Up
-- A link to choose a new password (#334). Only the hash of the token is kept, so a copy of
-- the database cannot be used to reset anyone's password. One link per account: asking
-- again replaces the earlier one. expires_at is a Unix time in seconds.
CREATE TABLE password_resets (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT    NOT NULL UNIQUE,
    expires_at INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE password_resets;
