# Conventions and security notes

Moved from CLAUDE.md. Read before writing user-facing text, naming things, or touching auth and sessions.

## Language

- **Code, docs, commits, issues and pull requests are in English** (#146): comments, doc comments, test names and messages, log messages, `CHANGELOG.md`, everything under `docs/`.
- **The text users see lives in translation catalogs** (#141): `internal/i18n/locales/<lang>/<area>.json`, Italian and English written together, `{{t "key"}}` in templates and `a.t(r, "key")` in handlers (see `docs/ARCHITECTURE.md`, Translations). New text goes into both catalogs, never straight into a template or handler.
- **The language** is the user's choice in Settings, else the `lang` cookie (the signed-out pages' selector), else the browser's `Accept-Language`, else English (#159). REST API errors are always English.
- History stays as written: commits, issues and pull requests before #146 are in Italian, and the CHANGELOG entries up to 2.0.0 link to them.
- `README.md` is English and `README.it.md` is its Italian version (it stays Italian): keep the two aligned.

## Code conventions

- **Commit style**: Conventional Commits with a scope, in English. Examples: `feat(shopping): add past weeks dropdown`, `fix(plan): correct cheat macro totals`
- **Handler names**: `handle{Section}{Action}` (e.g. `handleIngredientCreate`)
- **Aggregate names**: `{Entity}Full` for structs with loaded relations (e.g. `RecipeWithItems`)
- **Errors**: sentinel errors per domain (e.g. `ErrIngredientNotFound`), wrapped with `%w`
- **Validation**: `withinLen(s, max)` for UTF-8-aware lengths; `MaxNameLen=200`, `MaxDescriptionLen=2000`

## Build rules

- The `internal/db/migrations/*.sql` migrations are embedded and applied by goose at startup: nothing to generate
- After changing `web/tailwind/input.css` or `web/tailwind/tailwind.config.js` → `make tailwind`
- HTML templates and CSS are embedded in the binary → rebuild to see them in tests
- **Tailwind is pinned to v3.4.17** — do NOT upgrade to v4 without a dedicated task

## Security (operational notes)

- CSRF on every form (nosurf token embedded in every template)
- Per-IP rate limiting on login (`internal/app/rate_limit.go`)
- Every resource (recipes, plans, templates) is scoped to the authenticated user
- In prod: generate `NUTREEK_SESSION_KEY` with `openssl rand -hex 32`
