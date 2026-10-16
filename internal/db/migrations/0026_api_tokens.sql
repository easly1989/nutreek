-- +goose Up

-- Personal API tokens. The plaintext token ('nutreek_' prefix + 32 hex bytes)
-- is shown only once, at creation; the DB keeps only the SHA-256.
CREATE TABLE api_tokens (
    id           INTEGER PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at DATETIME
);
CREATE INDEX idx_api_tokens_user ON api_tokens(user_id);

-- +goose Down
DROP TABLE IF EXISTS api_tokens;
