-- +goose Up

-- Sottoscrizioni premium per utente. Predisposizione completa:
--   source      = come e' stata creata ('manual' dall'admin panel oggi;
--                 'license' o 'payment' in futuro senza cambi di schema)
--   license_key = chiave di licenza associata (futuro flusso Ed25519:
--                 l'attivazione di una licenza creera' semplicemente una
--                 riga qui, HasFeature non cambia)
--   expires_at  = NULL -> senza scadenza
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
