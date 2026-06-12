-- +goose Up

CREATE TABLE features (
    key         TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT,
    is_premium  INTEGER NOT NULL DEFAULT 0,
    enabled     INTEGER NOT NULL DEFAULT 1,
    sort_order  INTEGER NOT NULL DEFAULT 0
);

INSERT INTO features (key, name, description, is_premium, sort_order) VALUES
    ('tdee_adaptive',  'TDEE adattivo',          'Stima fabbisogno reale dal trend peso/intake', 1, 1),
    ('auto_plan',      'Generazione piano auto',  'Genera la settimana per centrare i target',    1, 2),
    ('coach_mode',     'Coach mode',              'Gestione multi-cliente per nutrizionisti',      1, 3),
    ('api_access',     'API REST',                'Accesso API con token + integrazioni',          1, 4),
    ('micronutrients', 'Micronutrienti',          'Visualizzazione fibre, zuccheri, saturi, sodio', 0, 5);

-- +goose Down
DROP TABLE IF EXISTS features;
