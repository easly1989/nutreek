# Nutreek — Project memory for Claude Code

## Overview

**Nutreek** is a self-hosted web app for planning weekly meals and tracking nutrition. Single developer (amon2126), open source under AGPL-3.0, active MVP. The README is in English (`README.md`) with an Italian version in `README.it.md`: keep the two aligned.

Main features: ingredients (local OpenFoodFacts search over FTS5), recipes, weekly templates, meal plans with macro totals (kcal, protein, carbs, fat), shopping list, cheat tracking, adaptive TDEE, automatic week generation, multi-client coach mode, REST API v1, admin panel with feature flags and premium subscriptions.
---

## Language

- **Code, docs, commits, issues and pull requests are in English** (#146): comments, doc comments, test names and messages, log messages, `CHANGELOG.md`, everything under `docs/`.
- **The text users see lives in translation catalogs** (#141): `internal/i18n/locales/<lang>/<area>.json`, Italian and English written together, `{{t "key"}}` in templates and `a.t(r, "key")` in handlers (see `docs/ARCHITECTURE.md`, Translations). New text goes into both catalogs, never straight into a template or handler.
- **The language** is the user's choice in Settings, else the `lang` cookie (the signed-out pages' selector), else the browser's `Accept-Language`, else English (#159). REST API errors are always English.
- History stays as written: commits, issues and pull requests before #146 are in Italian, and the CHANGELOG entries up to 2.0.0 link to them.
- `README.it.md` is the Italian README and stays Italian.

---
## Tech stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.27+, chi router |
| Database | SQLite (`modernc.org/sqlite`, pure Go, no CGO) |
| Queries | Hand-written SQL on `database/sql` (repositories in `internal/db/*.go`) |
| Migrations | Goose |
| Sessions | SCS v2 + SQLite store |
| Auth | bcrypt (cost 12), nosurf CSRF |
| Frontend | `html/template` + HTMX + Tailwind CSS v3.4.17 |
| Deploy | Static binary / Docker distroless (~20 MB) |

---
## Directory layout

Go app: `cmd/nutreek/` entrypoint; `internal/` holds `app` (handlers, router), `db` (repositories, goose
migrations), `service`, `planner`, `tdee`, `macros`, `off`, `i18n`, `web` (templates, static). Generated
`internal/web/static/css/tailwind.css` is never edited by hand. Full tree and "which files for which task":
[docs/file-map.md](docs/file-map.md).

---

## Main commands

```bash
make setup            # download Tailwind CLI v3.4.17 into ./bin/
make generate         # alias of tailwind (run after changing templates or CSS)
make tailwind         # CSS build only
make tailwind-watch   # watch CSS in dev
make dev              # go run ./cmd/nutreek  →  localhost:8080
make test             # go test ./...
make build            # static binary (CGO_ENABLED=0)
make docker-up        # docker compose up --build
```

---

## Reference docs (read when the task touches them)

- Environment variables (`NUTREEK_*`; `NUTREEK_SESSION_KEY` is required in prod): [docs/env-vars.md](docs/env-vars.md)
- OpenFoodFacts local FTS5 search and sync (all search is local, never call OFF search live): [docs/off-strategy.md](docs/off-strategy.md)
- Premium features, `HasFeature`, `requireFeature`, feature keys: [docs/premium-features.md](docs/premium-features.md)

---

## Code conventions

- **Language**: English for code, comments, commits and docs; see [Language](#language). User-facing text goes into the translation catalogs (#141).
- **Commit style**: Conventional Commits with a scope, in English
  Examples: `feat(shopping): add past weeks dropdown`, `fix(plan): correct cheat macro totals`
- **Handler names**: `handle{Section}{Action}` (e.g. `handleIngredientCreate`)
- **Aggregate names**: `{Entity}Full` for structs with loaded relations (e.g. `RecipeWithItems`)
- **Errors**: sentinel errors per domain (e.g. `ErrIngredientNotFound`), wrapped with `%w`
- **Validation**: `withinLen(s, max)` for UTF-8-aware lengths; `MaxNameLen=200`, `MaxDescriptionLen=2000`

### Build rules

- The `db/migrations/*.sql` migrations are embedded and applied by goose at startup: nothing to generate
- After changing `web/tailwind/input.css` or `tailwind.config.js` → `make tailwind`
- HTML templates and CSS are embedded in the binary → rebuild to see them in tests
- **Tailwind is pinned to v3.4.17** — do NOT upgrade to v4 without a dedicated task

---

## Testing

- `go test ./...` covers: auth, macros, the db layer (every repository), OFF client/reconciler, rate limiter, API tokens, features
- Pattern: table tests, `t.Cleanup()` for resources, a temporary SQLite DB for isolation
- GitHub Actions CI (`.github/workflows/ci.yml`): tests with `-race`, golangci-lint (`.golangci.yml`), binary and Docker image build, govulncheck
- Release (`.github/workflows/release.yml`): publishing a GitHub Release `vX.Y.Z` builds and pushes the public image `ghcr.io/easly1989/nutreek` tagged `:latest`, `:vX.Y.Z`, `:vX.Y`
- Demo (`.github/workflows/docker-demo.yml`): every published release (not prereleases) and a run by hand push `ghcr.io/easly1989/nutreek:demo` and `:demo-<version>`, then deploy it on Northflank. CI runs `go test -tags demo ./cmd/nutreek/`, an end-to-end test of the demo binary. Setup and secrets: [`docs/demo.md`](docs/demo.md)
- Manual UI testing: `make dev` → `localhost:8080`

---

## Branch workflow — always follow it

One issue → one branch → one PR to `main`, with `Closes #N` in the description.

1. A dedicated branch with a `feat/ fix/ chore/ ci/ refactor/ test/ docs/` prefix (e.g. `feat/shopping-past-weeks`)
2. Atomic commits, Conventional Commits in English
3. Push: `git push -u origin <branch>`, then a PR to `main`; CI must be green
4. Issues and PRs that touch the UI carry images (a mockup in the issue, real screenshots in the PR)
5. **Never push directly to `main`**

---
---

## Security (operational notes)

- CSRF on every form (nosurf token embedded in every template)
- Per-IP rate limiting on login (`internal/app/rate_limit.go`)
- Every resource (recipes, plans, templates) is scoped to the authenticated user
- In prod: generate `NUTREEK_SESSION_KEY` with `openssl rand -hex 32`

## Agent skills

### Issue tracker

GitHub Issues, via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Domain docs

Single-context: `CONTEXT.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.

### Token tools

CBM: call `index_repository` once, then `search_graph` / `trace_path` before reading files.
