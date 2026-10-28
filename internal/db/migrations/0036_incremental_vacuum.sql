-- +goose NO TRANSACTION
-- +goose Up
-- After a bootstrap the Syncer gives the freed pages back to the disk in
-- short steps (PRAGMA incremental_vacuum), so other writers never wait long
-- (#214). That needs auto_vacuum = INCREMENTAL, which an existing database
-- takes only through one full VACUUM: it runs here, at startup, before the
-- server accepts requests. One statement block, so both run on the same
-- connection of the pool.
-- +goose StatementBegin
PRAGMA auto_vacuum = INCREMENTAL;
VACUUM;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
PRAGMA auto_vacuum = NONE;
VACUUM;
-- +goose StatementEnd
