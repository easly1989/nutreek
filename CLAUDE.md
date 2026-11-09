# Nutreek — Project memory for Claude Code

Self-hosted web app for weekly meal planning and nutrition tracking (single developer, AGPL-3.0). Features: see `README.md`.

## Stack

Go 1.27+ with chi; SQLite via `modernc.org/sqlite` (pure Go, no CGO), hand-written SQL on `database/sql` (repositories in `internal/db/*.go`), Goose migrations; SCS v2 sessions; bcrypt (cost 12) + nosurf CSRF; `html/template` + HTMX + Tailwind CSS v3.4.17 (pinned: do NOT upgrade to v4 without a dedicated task); static binary / Docker distroless.

Layout: `cmd/nutreek/` entrypoint, `internal/` packages. Generated `internal/web/static/css/tailwind.css` is never edited by hand. Full tree and "which files for which task": [docs/file-map.md](docs/file-map.md).

## Commands

```bash
make setup            # download Tailwind CLI v3.4.17 into ./bin/
make tailwind         # CSS build; also `make generate`; run after changing templates or CSS
make dev              # go run ./cmd/nutreek  →  localhost:8080
make test             # go test ./...
make build            # static binary (CGO_ENABLED=0)
make docker-up        # docker compose up --build
```

Templates and CSS are embedded in the binary: rebuild to see them in tests. Migrations are embedded and applied by goose at startup.

## Rules

- Code, comments, docs, commits, issues and PRs are in English. User-visible text goes into both translation catalogs (`internal/i18n/locales/<lang>/<area>.json`), never straight into a template or handler. Keep `README.md` and `README.it.md` aligned. Details: [docs/agents/conventions.md](docs/agents/conventions.md).
- Conventional Commits with a scope (`feat(shopping): add past weeks dropdown`).
- Handlers `handle{Section}{Action}`; aggregates `{Entity}Full`; sentinel errors per domain wrapped with `%w`; `withinLen(s, max)` for lengths.
- One issue → one branch (`feat/ fix/ chore/ ci/ refactor/ test/ docs/`) → one PR to `main` with `Closes #N`; CI green; **never push to `main`**. UI issues and PRs carry images.
- Every resource is scoped to the authenticated user; `NUTREEK_SESSION_KEY` is required in prod.

## Read when the task touches it

- Environment variables (`NUTREEK_*`): [docs/env-vars.md](docs/env-vars.md)
- OpenFoodFacts local FTS5 search and sync (all search is local, never call OFF search live): [docs/off-strategy.md](docs/off-strategy.md)
- Premium features, `HasFeature`, `requireFeature`, feature keys: [docs/premium-features.md](docs/premium-features.md)
- Naming, validation limits, build rules, security notes: [docs/agents/conventions.md](docs/agents/conventions.md)
- Tests, CI, release, demo, branch workflow: [docs/agents/testing-and-release.md](docs/agents/testing-and-release.md)

## Agent skills

### Issue tracker

GitHub Issues, via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Domain docs

Single-context: `CONTEXT.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.

### Token tools

CBM: call `index_repository` once, then `search_graph` / `trace_path` before reading files.
