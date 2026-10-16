-- +goose Up
-- +goose StatementBegin

-- Schema required by alexedwards/scs/sqlite3store.
CREATE TABLE sessions (
    token  TEXT    PRIMARY KEY,
    data   BLOB    NOT NULL,
    expiry REAL    NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions(expiry);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS sessions;
-- +goose StatementEnd
