# Architecture — Nutreek

Technical reference for evaluating the project (due diligence) and for onboarding
a developer. It describes the structure, the main flows and the design decisions.

For running and configuring the app see [README.md](../README.md); for the
handover see [HANDOVER.md](HANDOVER.md).

---

## Overview

Nutreek is a self-hosted monolithic web application written in Go. Server-side
rendering with `html/template` + HTMX: no SPA, no complex frontend build.
Storage is embedded SQLite (pure Go, no CGO). The binary is static and carries
the templates, CSS and migrations through `embed`: a single executable with no
runtime dependencies.

**Project size** (application code, generated code excluded, at the time of writing):
- ~16,900 lines of application Go + ~4,450 lines of tests (32 files, 116 test functions)
- 127 HTTP routes, 42 HTML templates, 26 SQL migrations, 13 internal packages
- 7 direct dependencies, CGO disabled

---

## Request flow

```
HTTP request
   │
   ▼
chi router  (internal/app/router.go)
   │
   ▼
middleware stack  (internal/app/middleware.go)
   ├─ SCS session load/save        (cookie + sessions table)
   ├─ nosurf CSRF                   (exempt: /api/*, /admin/*)
   ├─ per-IP rate limiter           (internal/app/rate_limit.go, login)
   ├─ requireAuth                   (redirects to /login when signed out)
   ├─ requireFeature(key)           (premium gating, optional per route)
   └─ requireCoach / requireAdmin   (RBAC, optional per route)
   │
   ▼
handler  (internal/app/*_handlers.go)
   ├─ input validation              (withinLen, form/JSON parsing)
   ├─ call a service or repository
   └─ render a template  /  JSON response
   │
   ▼
service  (internal/service/*)         business logic orchestration
   │
   ▼
db repository  (internal/db/*.go)     hand-written SQL (database/sql)
   │
   ▼
SQLite  (modernc.org/sqlite, single file)
```

The "pure" packages (`macros`, `planner`, `tdee`, `domain`) have no side effects
and are tested in isolation; the `service` package combines them with the DB
repositories.

---

## Internal packages (`internal/`)

| Package | Responsibility |
|---------|----------------|
| `app/` | chi router, middleware, HTTP handlers, rate limiter, validation |
| `auth/` | Password hashing (bcrypt cost 12), session helpers |
| `config/` | Loading the `NUTREEK_*` environment variables |
| `db/` | One repository per domain; inline SQL, `db/migrations/` embedded and applied by goose |
| `domain/` | Plain domain structs (no business logic) |
| `i18n/` | The supported languages and the translation catalogs (`locales/<lang>/<area>.json`), loaded with go-i18n |
| `macros/` | Pure, tested macro calculations (kcal/protein/carbs/fat) |
| `off/` | OpenFoodFacts client, local dump sync, periodic reconciler |
| `planner/` | Greedy weekly plan generation |
| `profile/` | User profile calculations (BMI, BMR, macro targets) |
| `recipeimport/` | Recipe import from a URL (`schema.org/Recipe`, JSON-LD/microdata) |
| `service/` | Orchestration: `PlannerService`, `TDEEService` |
| `tdee/` | Adaptive TDEE estimate (pure algorithm over the weight/intake trend) |
| `web/` | HTML templates + static assets, embedded in the binary |

---

## Translations

The interface text lives in `internal/i18n/locales/<lang>/<area>.json`: one flat
object of keys per area (`layout`, `common`, `ingredients`, …), in the go-i18n v2
JSON format Weblate reads. Keys are English and start with their area
(`ingredients.new`); placeholders use the Go template syntax (`{{.Count}}`), and
an object of plural forms (`one`, `other`, …) replaces the string when a count
changes the text.

- Templates call `{{t "key"}}`, with placeholders as pairs:
  `{{t "ingredients.results.count" "Count" 3}}`. The renderer parses every page
  once per language, so `t` works in partials too.
- Handlers call `a.t(r, "key")`. Flash messages are stored as keys and translated
  when shown; validation errors are translated where they are built.
- Text a script shows comes from the page, translated, in `data-*` attributes.
- Reference data seeded by the migrations is translated by its stable code, not
  by the name stored in the database (`reference.json`): `{{ref "allergen" .Code .Name}}`
  and `{{ref "activity" .Code .Name}}` in templates; features by their key, in Go.
  A row without a code or a translation shows its stored name. User-entered
  names and OpenFoodFacts product names are never translated.
- A missing key falls back to English, then to the key itself. `TestCatalogs`
  fails when Italian or English lack a key, a value is empty, or the placeholders
  differ from English.
- The language comes from the `lang` cookie when it names a supported language,
  otherwise Italian. Nothing sets the cookie yet: until every area is translated
  (#141) it only serves to test English.

---

## Key flows

### Authentication and sessions
Sign-up and sign-in live in `internal/app/auth_handlers.go`; passwords use bcrypt
(`internal/auth`). Sessions use SCS v2 with a SQLite store (`sessions` table).
The middleware in `internal/app/middleware.go` loads the session, applies CSRF
(nosurf) and puts the user in the context. Every resource is scoped to the
authenticated user.

### Premium feature gating
The core of the monetization model. `HasFeature(ctx, user, key)` in
`internal/app/features.go` decides access:
- `NUTREEK_PREMIUM_ENABLED=false` → everything available (default);
- owner (`NUTREEK_OWNER_EMAIL`) → full access;
- disabled feature (kill switch) → blocked for everyone;
- non-premium feature → available;
- premium feature → needs an active subscription (`internal/db/subscriptions.go`).

The `requireFeature(key)` middleware protects routes (HTML → redirect to `/upgrade`,
HTMX → `HX-Redirect`, API → `403 JSON`). Feature definitions live in
`internal/db/features.go`; an in-memory cache (`sync.RWMutex`, 60 s TTL) avoids
repeated queries and is invalidated on every toggle from the admin panel.

### Automatic plan generation
`internal/planner/generator.go` implements a greedy algorithm that fills the
week from the variants of the nutrition plan to get close to the user's macro
targets. Orchestrated by `internal/service/planner_service.go`.

### Adaptive TDEE
`internal/tdee/estimator.go` estimates the real energy expenditure from the
weekly weight-vs-intake trend (data from `weight_logs` and the meal history).
`internal/service/tdee_service.go` exposes the calculation and the suggested
target update.

### OpenFoodFacts (local search)
`internal/off/dump.go` downloads the public OFF dump weekly, stream-parses it
keeping Italian products, and fills `off_products` and the
`off_products_fts` FTS5 index. Searches are fully local (<50 ms), with no HTTP
calls on the search path. Barcode lookup stays live against the OFF API
(`internal/off/client.go`), used only the first time a new product is imported.
Details and rationale in [CLAUDE.md](../CLAUDE.md#openfoodfacts-off-strategy).

### REST API v1
`internal/app/api_handlers.go` exposes JSON endpoints under `/api/v1/*`,
authenticated with a Bearer token. Tokens are generated as `nutreek_<hex>` and
stored as SHA-256 hashes (`internal/db/api_tokens.go`). The route group is
protected by `requireAPIToken` + `requireFeature("api_access")`.

### Coach mode (RBAC)
Users with the `coach` role reach the `/coach/*` group
(`internal/app/coach_handlers.go`). They manage a library of nutrition plans
and assign them to linked clients (`internal/db/coach.go`, `coach_clients`
table). Protected by `requireCoach` + `requireFeature("coach_mode")`.

### Admin panel
`internal/app/admin_panel_handlers.go` serves `/admin/panel`, reachable only by
the owner (the email in `NUTREEK_OWNER_EMAIL`). It manages feature flags, the
user list and premium subscriptions.

---

## Data model (high level)

Main tables grouped by domain (full detail in `internal/db/migrations/`):

- **Users & sessions**: `users` (with profile, role, macro targets), `sessions`
- **Plans & meals**: `weekly_plans`, `meals`, `meal_items`,
  `meal_item_customizations`, `meal_cheat_items`, `meal_templates`,
  `meal_template_slots`
- **Nutrition plan**: `nutritionist_plans`, `nutritionist_meal_types`,
  `nutritionist_meal_variations`, `nutritionist_meal_items`
- **Ingredients & OFF**: `ingredients`, `ingredient_categories`,
  `ingredient_user_categories`, `allergen_tags`, `ingredient_allergens`,
  `off_products`, `off_products_fts` (FTS5), `off_sync_state`
- **Recipes**: `recipes`, `recipe_ingredients`
- **Tracking**: `weight_logs`, `water_logs`, `workout_activities`, `workout_logs`
- **Shopping list**: `shopping_lists`, `shopping_list_items`, `shopping_user_items`
- **Coach**: `coach_clients`
- **Monetization**: `features`, `subscriptions`, `api_tokens`

Goose migrations are embedded and run automatically at startup.

---

## Notable design decisions

- **Pure-Go SQLite (no CGO)**: `modernc.org/sqlite`. Static build
  (`CGO_ENABLED=0`), trivial cross-compiling, a ~20 MB distroless Docker image.
- **Local OFF instead of runtime calls**: the public OFF endpoints are not
  reliable for search-as-you-type (rate limit, latency, ~20% 503s). Dropping
  the runtime dependency is the only fully reliable way.
- **Embedded templates, CSS and migrations**: a single binary with no external
  files to deploy; the Tailwind CSS is generated at build time and must be
  regenerated after changes.
- **60 s feature cache**: premium gating sits on every request's path; the
  cache avoids repeated DB queries while staying responsive to admin toggles
  (invalidation).
- **Server-side + HTMX**: no frontend build, no SPA to maintain; interactivity
  through HTMX and small vanilla scripts.

---

## Tests and quality

- `go test ./...` is green. Covered: auth, macros, the db layer (every
  repository), OFF client/reconciler, rate limiter, API tokens, features,
  planner, tdee, ownership checks on handlers.
- Pattern: table tests, `t.Cleanup()`, a temporary SQLite DB for isolation.
- GitHub Actions CI runs the tests with `-race`, golangci-lint, the binary and
  Docker builds and govulncheck on every pull request.
- **Not present**: end-to-end/UI tests.
