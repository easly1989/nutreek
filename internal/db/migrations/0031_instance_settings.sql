-- +goose Up
-- Settings of the whole installation, chosen by the owner (#167): one row per
-- key. The first is whether the server looks for newer Nutreek versions.
CREATE TABLE instance_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- +goose Down
DROP TABLE instance_settings;
