-- +goose Up
-- The version of the dump parser that filled off_products (#204). When the
-- code's version (off.ParserVersion) differs, the Syncer downloads the dump
-- again even if its ETag has not changed. 0 = before this column existed.
ALTER TABLE off_sync_state ADD COLUMN parser_version INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE off_sync_state DROP COLUMN parser_version;
