-- +goose Up
-- Who a person is at a sign-in provider (#333): the provider's issuer and the person's
-- subject there. After the first sign-in an account is found by this, never by email, so a
-- changed email at the provider cannot hand someone else's account to another person.
CREATE TABLE user_identities (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    issuer     TEXT    NOT NULL,
    subject    TEXT    NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (issuer, subject),
    UNIQUE (user_id, issuer)
);

-- +goose Down
DROP TABLE user_identities;
