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

```
cmd/nutreek/          → entrypoint (main.go, -healthcheck subcommand)
internal/
  app/                → HTTP handlers, router (chi), middleware, rate limiter, validation
  auth/               → password hashing (bcrypt), session helpers
  config/             → environment variables (NUTREEK_*)
  db/
    migrations/       → goose SQL (0001…0028+, embedded)
  demo/               → public demo (#137): throwaway accounts, only in the `-tags demo` build
  domain/             → plain domain structs (no business logic)
  macros/             → pure macro calculations (no side effects, tested)
  off/                → OpenFoodFacts client + local dump sync + periodic reconciler
  planner/            → greedy weekly plan generation (GenerateWeek)
  profile/            → user profile logic (BMI, BMR, macro targets)
  recipeimport/       → recipe import from a URL (schema.org/Recipe, JSON-LD/microdata)
  service/            → orchestration: PlannerService (GenerateWeek, EffectivePlanFull),
                        TDEEService (adaptive TDEE from weight/intake trends)
  tdee/               → pure adaptive TDEE algorithm (no side effects)
  web/
    templates/        → HTML with HTMX (embedded in the binary at compile time)
    static/css/       → generated tailwind.css — do NOT edit by hand
web/tailwind/         → tailwind.config.js + input.css (CSS sources)
scripts/              → PowerShell helpers for Windows
data/                 → SQLite DB at runtime (gitignored)
```

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

## Environment variables

| Variable | Default | Notes |
|-----------|---------|------|
| `NUTREEK_ADDR` | `:8080` | |
| `NUTREEK_DB_PATH` | `./data/nutreek.db` | |
| `NUTREEK_ENV` | `dev` | `prod` enables secure cookies |
| `NUTREEK_SESSION_KEY` | auto in dev | **required in prod** (hex ≥64 chars) |
| `NUTREEK_OFF_BASE_URL` | `https://world.openfoodfacts.org` | live product lookup host (barcode) |
| `NUTREEK_OFF_CSV_URL` | `https://static.openfoodfacts.org/data/en.openfoodfacts.org.products.csv.gz` | CSV export the Syncer bootstraps from |
| `NUTREEK_OFF_DELTA_URL` | `https://static.openfoodfacts.org/data/delta/` | folder of `index.txt` and of the daily deltas |
| `NUTREEK_OFF_SYNC_INTERVAL` | `24h` | minimum interval of the automatic sync |
| `NUTREEK_OFF_SYNC_ENABLED` | `true` | enables the sync scheduler |
| `NUTREEK_RECONCILE_INTERVAL` | `24h` | |
| `NUTREEK_RECONCILE_ENABLED` | `true` | |
| `NUTREEK_ADMIN_TOKEN` | (none) | token for the legacy admin endpoints |
| `NUTREEK_OWNER_EMAIL` | (none) | owner's email — sole access to `/admin/panel` |
| `NUTREEK_PREMIUM_ENABLED` | `false` | when `true`, gates the features marked premium |
| `NUTREEK_BACKUP_ENABLED` | `false` | enables periodic automatic SQLite backups |
| `NUTREEK_BACKUP_DIR` | `./data/backups` | where backups are written |
| `NUTREEK_BACKUP_INTERVAL` | `24h` | automatic backup interval |
| `NUTREEK_BACKUP_KEEP` | `7` | backups to keep (the oldest are deleted) |
| `NUTREEK_UPDATE_CHECK` | `true` | release builds read `versions.json` (branch `site-data`) once a day and tell the owner of a newer release; the owner's switch lives in `instance_settings` |
| `NUTREEK_DEMO_IDLE` | `2h` | demo build only: a demo session ends, and its group is deleted, after this long without use; every group is deleted nightly at 03:00 UTC |
| `NUTREEK_DEMO_MAX_USERS` | `200` | demo build only: live demo users; then the entry says the demo is full |
| `NUTREEK_DEMO_STARTS_PER_HOUR` | `10` | demo build only: demos started per client address per hour; then 429 |
| `NUTREEK_DEMO_MAX_BODY` | `1048576` | demo build only: request body in bytes; then 413 |
| `NUTREEK_DEMO_MAX_INGREDIENTS` | `300` | demo build only: hand-made ingredients per user |
| `NUTREEK_DEMO_MAX_RECIPES` | `100` | demo build only: recipes per user |

## OpenFoodFacts (OFF) strategy

All OFF text search is **local**, via SQLite FTS5. The `Syncer`
(`internal/off/dump.go`, `bootstrap.go`, `delta.go`) bootstraps from
OFF's CSV export (~0.8 GB compressed), stream-parses it keeping
Italian products (`countries_tags` contains `en:italy`, ~250k
products) into `off_products_staging`, then swaps that into
`off_products` (and its `off_products_fts` FTS5 index) in
transactions of 5,000 rows with a short pause between them, so no
write lock nears the 5 s `busy_timeout`. Searches keep the old products while the export
downloads and may see a mix during the swap; a failed download
leaves them in place. The swap is refused when more than 1% of the
rows are malformed or staging holds under 80% of the current
products. The state is written after the swap, so a swap that fails
midway bootstraps again. It records the high-water mark, the newest
`last_modified_t` of the export (never later than now). Then,
daily, it reads `delta/index.txt` and applies, oldest first, every
delta (one day of changed products, the dump's JSONL shape) that ends
after the high-water mark: Italian products are upserted, local ones
no longer Italian deleted. Result: searches under 50 ms, no HTTP
calls on the search path, no more 503s.

A sync bootstraps from the CSV again when the deltas no longer reach
the high-water mark (offline for more than ~13 days), when
`off.ParserVersion` differs from the stored one (bump it when a parser
change must reach the imported data), or when the last bootstrap is
older than 30 days (deltas never list deleted products). When the
export is older than every delta, it applies them all anyway and
moves the high-water mark on, rather than bootstrapping again daily.

Why: the public OFF endpoints are not reliable for search-as-you-type.
`cgi/search.pl` has an official rate limit of 10 req/min, 3–12 s
latency and ~20% 503s. Search-a-licious has indexing gaps. The only
fully reliable way is to drop the runtime dependency on OFF search.

Barcode lookup (`OFF.GetByBarcode`) stays live against
`world.openfoodfacts.org/api/v2/product/{code}.json`: a single-product
endpoint, reliable, used only the first time a new product is imported.

Manual trigger: `POST /admin/refresh-off-dump` (needs
`X-Admin-Token`) forces a CSV bootstrap. Current state:
`GET /admin/off-status`.

**First start**: `off_products` is empty; the scheduler starts the
bootstrap in the background. Searches return nothing until it
completes (~10 min, ~0.8 GB of bandwidth).

---

## Premium features

### `HasFeature` logic

`app.HasFeature(ctx, user, key) bool` — the single entry point for premium access:

- `PremiumEnabled=false` → **always `true`** (everything available, dev default)
- The user is the owner (`isOwner`) → **always `true`**
- `feature.enabled = false` → **`false`** for everyone (kill switch)
- `feature.is_premium = false` → **`true`** (public feature, enabled)
- `feature.is_premium = true` → **`Subscriptions.IsActive(userID)`**
- DB error or unknown feature → `true` (permissive fallback)

### `requireFeature` middleware

`app.requireFeature(key)` — chi middleware that protects routes:

- Browser HTML request → `302 /upgrade`
- HTMX request (`HX-Request: true`) → `HX-Redirect: /upgrade` + `403`
- API/JSON request → `403 {"error": "feature not available on your plan"}`

### Cache

`featureCache` — `sync.RWMutex` + 60 s TTL, invalidated after every admin toggle (`invalidateFeatureCache()`).

### Feature keys in use

| Key | Feature | Protected routes |
|-----|-------------|---------------|
| `auto_plan` | Automatic week generation | `POST /week/generate` |
| `tdee_adaptive` | Adaptive TDEE | `POST /tdee/accept` |
| `coach_mode` | Coach panel | group `/coach/*` |
| `api_access` | REST API v1 | group `/api/v1/*`, `POST /settings/api-tokens` |

### `Features` in templates

`authPageData.Features map[string]bool` — filled in `baseData()` on every authenticated request (cheap: uses the 60 s cache). Templates use `{{if index .Features "key"}}` to show Premium badges and upgrade messages before the user clicks.

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

## Quick guide: files for common tasks

| Task | Main files |
|------|----------------|
| New endpoint | `internal/app/router.go` + `internal/app/*_handlers.go` |
| New DB query | a method on the repository in `internal/db/*.go` (inline SQL) + test |
| New DB migration | `internal/db/migrations/000N_*.sql` (goose runs it at startup) |
| New domain model | `internal/domain/*.go` |
| New HTML page | `internal/web/templates/pages/*.html` + renderer if needed |
| New HTMX partial | `internal/web/templates/partials/*.html` |
| CSS change / new theme | `web/tailwind/input.css` → `make tailwind` |
| Business logic | `internal/service/*.go` |
| Macro calculations | `internal/macros/macros.go` |
| OpenFoodFacts integration | `internal/off/client.go`, `internal/off/reconciler.go` |
| Feature flags | `internal/db/features.go`, `internal/app/features.go` |
| Subscriptions | `internal/db/subscriptions.go`, migration `0024_*` |
| Coach mode | `internal/db/coach.go`, `internal/app/coach_handlers.go` |
| REST API | `internal/app/api_handlers.go`, `internal/db/api_tokens.go` |
| Admin panel | `internal/app/admin_panel_handlers.go` |
| Translations | `internal/i18n/locales/<lang>/<area>.json`, `internal/i18n/i18n.go` |
| Plan generation | `internal/planner/`, `internal/service/planner.go` |
| Adaptive TDEE | `internal/tdee/`, `internal/service/tdee.go` |
| Recipe import from URL | `internal/recipeimport/` |

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
