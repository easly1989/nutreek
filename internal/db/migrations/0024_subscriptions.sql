-- +goose Up

-- Premium subscriptions per user. Fully prepared:
--   source      = how it was created ('manual' from the admin panel today;
--                 'license' or 'payment' later, with no schema change)
--   license_key = the license key attached (a future Ed25519 flow:
--                 activating a license will simply create a row here,
--                 HasFeature does not change)
--   expires_at  = NULL -> no expiry
CREATE TABLE subscriptions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    plan        TEXT NOT NULL DEFAULT 'premium' CHECK(plan IN ('premium')),
    status      TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','revoked')),
    source      TEXT NOT NULL DEFAULT 'manual' CHECK(source IN ('manual','license','payment')),
    license_key TEXT,
    started_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at  TIMESTAMP,
    notes       TEXT
);

-- +goose Down
DROP TABLE IF EXISTS subscriptions;
