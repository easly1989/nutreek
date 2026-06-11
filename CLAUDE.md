# Nutreek — Memoria per Claude Code

## Panoramica progetto

**Nutreek** è un'app web self-hosted per la gestione di piani nutrizionali settimanali, scritta interamente in italiano. Sviluppatore singolo (amon2126), licenza proprietaria, MVP attivo.

Funzionalità principali: gestione ingredienti (integrazione OpenFoodFacts), ricette, template settimanali, piani pasto con calcolo macro (Kcal, Proteine, Carboidrati, Grassi), lista della spesa, tracciamento sgarri.

---

## Stack tecnico

| Layer | Tecnologia |
|-------|-----------|
| Backend | Go 1.25+, chi router |
| Database | SQLite (`modernc.org/sqlite`, pure Go, no CGO) |
| ORM/Query | SQLC (codegen da SQL) |
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
    migrations/       → SQL goose (0001…0005, embedded)
    queries/          → SQL input per sqlc
    sqlc/             → codice Go generato da sqlc — NON editare a mano
  domain/             → struct di dominio pure (nessuna business logic)
  macros/             → calcoli macro puri (zero side-effect, testati)
  off/                → client OpenFoodFacts + reconciler periodico
  service/            → orchestrazione business logic (in espansione)
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
make generate         # sqlc + tailwind (eseguire dopo modifiche SQL o CSS)
make tailwind         # solo build CSS
make tailwind-watch   # watch CSS in dev
make dev              # go run ./cmd/nutreek  →  localhost:8080
make test             # go test ./...
make build            # binary statico (CGO_ENABLED=0)
make docker-up        # docker compose up --build
make sqlc             # solo rigenera sqlc
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
| `NUTREEK_ADMIN_TOKEN` | (nessuno) | token per admin endpoints |

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

## Convenzioni codice

- **Lingua**: italiano per commenti, UI, messaggi di commit
- **Commit style**: Conventional Commits con scope  
  Esempi: `feat(shopping): aggiungi dropdown settimane passate`, `fix(plan): correggi calcolo macro cheat`
- **Nomi handler**: `handle{Sezione}{Azione}` (es. `handleIngredientCreate`)
- **Nomi aggregati**: `{Entity}Full` per struct con relazioni caricate (es. `RecipeWithItems`)
- **Errori**: sentinel errors per dominio (es. `ErrIngredientNotFound`), wrapping con `%w`
- **Validazione**: `withinLen(s, max)` per lunghezze UTF-8 aware; `MaxNameLen=200`, `MaxDescriptionLen=2000`

### Regole build

- Dopo modifiche a `db/queries/*.sql` o `db/migrations/*.sql` → `make generate` (sqlc + tailwind)
- Dopo modifiche a `web/tailwind/input.css` o `tailwind.config.js` → `make tailwind`
- I template HTML e il CSS sono embedded nel binary → serve rebuild per vederli nei test
- **Tailwind bloccato a v3.4.17** — NON aggiornare a v4 senza task dedicato

---

## Testing

- `go test ./...` copre: auth, macros, db layer (tutti i repo), OFF client/reconciler, rate limiter
- Pattern: tabella di test, `t.Cleanup()` per risorse, SQLite temporaneo per isolamento
- Nessun linter configurato, nessuna CI/CD
- Test UI manuali: `make dev` → `localhost:8080`

---

## Workflow branch — da seguire sempre

1. Creare branch dedicato: `feat/<scope>-<breve-descrizione>` o `fix/<scope>-<breve-descrizione>`
2. Sviluppare il task sul branch
3. Commit atomici con conventional commits in italiano
4. Push: `git push -u origin <branch>`
5. Aprire merge request verso `main`
6. **NON fare push diretto su `main`**

---

## Guida rapida ai file per task comuni

| Task | File principali |
|------|----------------|
| Nuovo endpoint | `internal/app/router.go` + `internal/app/*_handlers.go` |
| Nuova query DB | `internal/db/queries/*.sql` → `make sqlc` → `internal/db/*.go` |
| Nuova migrazione DB | `internal/db/migrations/000N_*.sql` (goose auto-run all'avvio) |
| Nuovo modello di dominio | `internal/domain/*.go` |
| Nuova pagina HTML | `internal/web/templates/pages/*.html` + renderer se serve |
| Nuovo partial HTMX | `internal/web/templates/partials/*.html` |
| Modifica CSS / nuovo tema | `web/tailwind/input.css` → `make tailwind` |
| Business logic | `internal/service/*.go` |
| Calcoli macro | `internal/macros/macros.go` |
| Integrazione OpenFoodFacts | `internal/off/client.go`, `internal/off/reconciler.go` |

---

## Sicurezza (note operative)

- CSRF attivo su tutti i form (token nosurf embedded in ogni template)
- Rate limiting per IP sul login (`internal/app/rate_limit.go`)
- Ogni risorsa (ricette, piani, template) è scoped all'utente autenticato
- In prod: `NUTREEK_SESSION_KEY` deve essere generato con `openssl rand -hex 32`
