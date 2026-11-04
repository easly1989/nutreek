# Directory layout and files for common tasks

Moved from CLAUDE.md. Read to find where a change belongs.

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
