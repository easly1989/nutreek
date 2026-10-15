# Nutreek — Memoria per Claude Code

## Panoramica progetto

**Nutreek** è un'app web self-hosted per la gestione di piani nutrizionali settimanali, scritta interamente in italiano. Sviluppatore singolo (amon2126), licenza proprietaria, MVP attivo.

Funzionalità principali: gestione ingredienti (integrazione OpenFoodFacts locale FTS5), ricette, template settimanali, piani pasto con calcolo macro (Kcal, Proteine, Carboidrati, Grassi), lista della spesa, tracciamento sgarri, TDEE adattivo, generazione automatica settimana, coach mode multi-cliente, REST API v1, admin panel con feature flag e sottoscrizioni premium.

---

## Stack tecnico

| Layer | Tecnologia |
|-------|-----------|
| Backend | Go 1.25+, chi router |
| Database | SQLite (`modernc.org/sqlite`, pure Go, no CGO) |
| Query | SQL scritto a mano su `database/sql` (repo in `internal/db/*.go`) |
| Migrations | Goose |
| Sessions | SCS v2 + SQLite store |
| Auth | bcrypt (cost 12), NOSURF CSRF |
| Frontend | `html/template` + HTMX + Tailwind CSS v3.4.17 |
| Deploy | Static binary / Docker distroless (~20 MB) |

---

## Struttura directory

```
cmd/nutreek/          → entrypoint (main.go, -healthcheck subcommand)
internal/
  app/                → handler HTTP, router (chi), middleware, rate limiter, validazione
  auth/               → hashing password (bcrypt), helper sessioni
  config/             → caricamento variabili d'ambiente (NUTREEK_*)
  db/
    migrations/       → SQL goose (0001…0025+, embedded)
  domain/             → struct di dominio pure (nessuna business logic)
  macros/             → calcoli macro puri (zero side-effect, testati)
  off/                → client OpenFoodFacts + sync dump locale + reconciler periodico
  planner/            → algoritmo greedy generazione piano settimanale (GenerateWeek)
  profile/            → logica salvataggio profilo utente (BMI, BMR, target macro)
  recipeimport/       → import ricetta da URL (schema.org/Recipe, JSON-LD/microdata)
  service/            → orchestrazione: PlannerService (GenerateWeek, EffectivePlanFull),
                        TDEEService (calcolo TDEE adattivo da trend peso/intake)
  tdee/               → calcolo TDEE adattivo puro (algoritmo, nessun side-effect)
  web/
    templates/        → HTML con HTMX (embedded nel binary al compile time)
    static/css/       → tailwind.css generato — NON editare direttamente
web/tailwind/         → tailwind.config.js + input.css (sorgenti CSS)
scripts/              → PowerShell helpers per Windows
data/                 → SQLite DB a runtime (gitignored)
```

---

## Comandi principali

```bash
make setup            # scarica Tailwind CLI v3.4.17 in ./bin/
make generate         # alias di tailwind (eseguire dopo modifiche a template o CSS)
make tailwind         # solo build CSS
make tailwind-watch   # watch CSS in dev
make dev              # go run ./cmd/nutreek  →  localhost:8080
make test             # go test ./...
make build            # binary statico (CGO_ENABLED=0)
make docker-up        # docker compose up --build
```

---

## Variabili d'ambiente

| Variabile | Default | Note |
|-----------|---------|------|
| `NUTREEK_ADDR` | `:8080` | |
| `NUTREEK_DB_PATH` | `./data/nutreek.db` | |
| `NUTREEK_ENV` | `dev` | `prod` abilita secure cookies |
| `NUTREEK_SESSION_KEY` | auto in dev | **richiesto in prod** (hex ≥64 chars) |
| `NUTREEK_OFF_BASE_URL` | `https://world.openfoodfacts.org` | host product lookup (barcode) live |
| `NUTREEK_OFF_DUMP_URL` | `https://static.openfoodfacts.org/data/openfoodfacts-products.jsonl.gz` | URL del dump JSONL.gz scaricato dal Syncer |
| `NUTREEK_OFF_SYNC_INTERVAL` | `168h` (7 giorni) | frequenza minima del sync automatico |
| `NUTREEK_OFF_SYNC_ENABLED` | `true` | abilita lo scheduler del sync |
| `NUTREEK_RECONCILE_INTERVAL` | `24h` | |
| `NUTREEK_RECONCILE_ENABLED` | `true` | |
| `NUTREEK_ADMIN_TOKEN` | (nessuno) | token per admin endpoints legacy |
| `NUTREEK_OWNER_EMAIL` | (nessuno) | email utente owner — accesso esclusivo a `/admin/panel` |
| `NUTREEK_PREMIUM_ENABLED` | `false` | se `true`, attiva il gating premium sulle feature marcate come tali |
| `NUTREEK_BACKUP_ENABLED` | `false` | abilita backup SQLite automatico periodico |
| `NUTREEK_BACKUP_DIR` | `./data/backups` | directory dove salvare i backup |
| `NUTREEK_BACKUP_INTERVAL` | `24h` | frequenza backup automatici |
| `NUTREEK_BACKUP_KEEP` | `7` | numero di backup da conservare (i più vecchi vengono eliminati) |

## Strategia OpenFoodFacts (OFF)

Tutta la ricerca testuale OFF e' **locale** via SQLite FTS5. Il `Syncer`
(`internal/off/dump.go`) scarica settimanalmente il dump pubblico OFF
(~0.9GB compressi), lo stream-parsa filtrando i prodotti italiani
(`countries_tags` contiene `en:italy`, ~250k prodotti) e popola la
tabella `off_products` + indice FTS5 `off_products_fts`. Risultato:
ricerche <50ms, zero chiamate HTTP in search-path, mai piu' 503.

Motivazione: gli endpoint pubblici OFF non sono affidabili per
search-as-you-type. `cgi/search.pl` ha rate limit ufficiale 10 req/min
+ latenza 3-12s + ~20% 503. Search-a-licious ha gap d'indicizzazione.
L'unica via 100% affidabile e' eliminare la dipendenza runtime dalla
ricerca OFF.

Lookup per barcode (`OFF.GetByBarcode`) resta live verso
`world.openfoodfacts.org/api/v2/product/{code}.json`: endpoint
puntuale, affidabile, usato solo al primo import di un prodotto nuovo.

Trigger manuale: `POST /admin/refresh-off-dump` (richiede
`X-Admin-Token`). Stato corrente: `GET /admin/off-status`.

**Bootstrap**: al primo avvio `off_products` e' vuota; lo scheduler
lancia un sync iniziale in background. Le ricerche restano vuote
finche' il sync non completa (~10 min, ~0.9GB di banda).

---

## Feature Premium

### Logica `HasFeature`

`app.HasFeature(ctx, user, key) bool` — funzione centrale per l'accesso premium:

- `PremiumEnabled=false` → **sempre `true`** (tutto accessibile, default dev)
- Utente è owner (`isOwner`) → **sempre `true`**
- `feature.enabled = false` → **`false`** per tutti (kill-switch)
- `feature.is_premium = false` → **`true`** (feature pubblica abilitata)
- `feature.is_premium = true` → **`Subscriptions.IsActive(userID)`**
- Errore DB o feature sconosciuta → `true` (fallback permissivo)

### Middleware `requireFeature`

`app.requireFeature(key)` — middleware chi per proteggere le route:

- Richiesta HTML browser → `302 /upgrade`
- Richiesta HTMX (`HX-Request: true`) → `HX-Redirect: /upgrade` + `403`
- Richiesta API/JSON → `403 {"error": "funzionalità non disponibile nel tuo piano"}`

### Cache

`featureCache` — `sync.RWMutex` + TTL 60s, invaldata automaticamente dopo ogni toggle admin (`invalidateFeatureCache()`).

### Feature keys in uso

| Key | Funzionalità | Route protette |
|-----|-------------|---------------|
| `auto_plan` | Generazione automatica settimana | `POST /week/generate` |
| `tdee_adaptive` | TDEE adattivo | `POST /tdee/accept` |
| `coach_mode` | Pannello coach | gruppo `/coach/*` |
| `api_access` | REST API v1 | gruppo `/api/v1/*`, `POST /settings/api-tokens` |

### `Features` nei template

`authPageData.Features map[string]bool` — popolato in `baseData()` per ogni richiesta autenticata (cheap: usa cache 60s). Usato nei template con `{{if index .Features "key"}}` per mostrare badge Premium e messaggi upgrade prima che l'utente clicchi.

---

## Convenzioni codice

- **Lingua**: italiano per commenti, UI, messaggi di commit
- **Commit style**: Conventional Commits con scope  
  Esempi: `feat(shopping): aggiungi dropdown settimane passate`, `fix(plan): correggi calcolo macro cheat`
- **Nomi handler**: `handle{Sezione}{Azione}` (es. `handleIngredientCreate`)
- **Nomi aggregati**: `{Entity}Full` per struct con relazioni caricate (es. `RecipeWithItems`)
- **Errori**: sentinel errors per dominio (es. `ErrIngredientNotFound`), wrapping con `%w`
- **Validazione**: `withinLen(s, max)` per lunghezze UTF-8 aware; `MaxNameLen=200`, `MaxDescriptionLen=2000`

### Regole build

- Le migrazioni `db/migrations/*.sql` sono embedded e applicate da goose all'avvio: nessuna generazione
- Dopo modifiche a `web/tailwind/input.css` o `tailwind.config.js` → `make tailwind`
- I template HTML e il CSS sono embedded nel binary → serve rebuild per vederli nei test
- **Tailwind bloccato a v3.4.17** — NON aggiornare a v4 senza task dedicato

---

## Testing

- `go test ./...` copre: auth, macros, db layer (tutti i repo), OFF client/reconciler, rate limiter, API tokens, features
- Pattern: tabella di test, `t.Cleanup()` per risorse, SQLite temporaneo per isolamento
- CI GitHub Actions (`.github/workflows/ci.yml`): test `-race`, golangci-lint (`.golangci.yml`), build binario + immagine Docker, govulncheck
- Release (`.github/workflows/release.yml`): pubblicare una GitHub Release `vX.Y.Z` costruisce e pusha `ghcr.io/easly1989/nutreek` (privata) con tag `:latest`, `:vX.Y.Z`, `:vX.Y`
- Test UI manuali: `make dev` → `localhost:8080`

---

## Workflow branch — da seguire sempre

Una issue → un branch → una PR verso `main`, con `Closes #N` nella descrizione.

1. Branch dedicato con prefisso `feat/ fix/ chore/ ci/ refactor/ test/ docs/` (es. `feat/shopping-settimane-passate`)
2. Commit atomici con conventional commits in italiano
3. Push: `git push -u origin <branch>`, poi PR verso `main`; la CI deve essere verde
4. Issue e PR che toccano l'interfaccia portano immagini (mockup nell'issue, screenshot reali nella PR)
5. **NON fare push diretto su `main`**

---

## Guida rapida ai file per task comuni

| Task | File principali |
|------|----------------|
| Nuovo endpoint | `internal/app/router.go` + `internal/app/*_handlers.go` |
| Nuova query DB | metodo sul repo in `internal/db/*.go` (SQL inline) + test |
| Nuova migrazione DB | `internal/db/migrations/000N_*.sql` (goose auto-run all'avvio) |
| Nuovo modello di dominio | `internal/domain/*.go` |
| Nuova pagina HTML | `internal/web/templates/pages/*.html` + renderer se serve |
| Nuovo partial HTMX | `internal/web/templates/partials/*.html` |
| Modifica CSS / nuovo tema | `web/tailwind/input.css` → `make tailwind` |
| Business logic | `internal/service/*.go` |
| Calcoli macro | `internal/macros/macros.go` |
| Integrazione OpenFoodFacts | `internal/off/client.go`, `internal/off/reconciler.go` |
| Feature flags | `internal/db/features.go`, `internal/app/features.go` |
| Subscriptions | `internal/db/subscriptions.go`, migration `0024_*` |
| Coach mode | `internal/db/coach.go`, `internal/app/coach_handlers.go` |
| REST API | `internal/app/api_handlers.go`, `internal/db/api_tokens.go` |
| Admin panel | `internal/app/admin_panel_handlers.go` |
| Generazione piano | `internal/planner/`, `internal/service/planner.go` |
| TDEE adattivo | `internal/tdee/`, `internal/service/tdee.go` |
| Import ricetta da URL | `internal/recipeimport/` |

---

## Sicurezza (note operative)

- CSRF attivo su tutti i form (token nosurf embedded in ogni template)
- Rate limiting per IP sul login (`internal/app/rate_limit.go`)
- Ogni risorsa (ricette, piani, template) è scoped all'utente autenticato
- In prod: `NUTREEK_SESSION_KEY` deve essere generato con `openssl rand -hex 32`

## Agent skills

### Issue tracker

GitHub Issues, via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Domain docs

Single-context: `CONTEXT.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.

### Token tools

CBM: call `index_repository` once, then `search_graph` / `trace_path` before reading files.
