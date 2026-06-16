# Architettura — Nutreek

Documento tecnico di riferimento per la valutazione (due-diligence) e l'onboarding
di uno sviluppatore. Descrive struttura, flussi principali e decisioni di design.

Per l'esecuzione e la configurazione vedi [README.md](../README.md); per il
passaggio di consegne vedi [HANDOVER.md](HANDOVER.md).

---

## Panoramica

Nutreek è un'applicazione web monolitica self-hosted scritta in Go. Server-side
rendering con `html/template` + HTMX; nessuna SPA, nessun build frontend
complesso. Persistenza su SQLite embedded (pure Go, no CGO). Il binario è statico
e include template, CSS e migrazioni via `embed` — un singolo file eseguibile
senza dipendenze esterne a runtime.

**Numeri del progetto** (codice applicativo, escluso generato):
- ~16.900 righe Go applicative + ~4.450 righe di test (32 file, 116 funzioni di test)
- 127 route HTTP, 42 template HTML, 26 migrazioni SQL, 13 package interni
- 7 dipendenze dirette, CGO disabilitato

---

## Flusso di una richiesta

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
   ├─ rate limiter per-IP           (internal/app/rate_limit.go, login)
   ├─ requireAuth                   (redirect /login se non loggato)
   ├─ requireFeature(key)           (gating premium, opzionale per route)
   └─ requireCoach / requireAdmin   (RBAC, opzionale per route)
   │
   ▼
handler  (internal/app/*_handlers.go)
   ├─ validazione input             (withinLen, parsing form/JSON)
   ├─ chiamata a service o repo
   └─ render template  /  risposta JSON
   │
   ▼
service  (internal/service/*)         orchestrazione business logic
   │
   ▼
db repo  (internal/db/*.go)           query via codice sqlc-generated
   │
   ▼
SQLite  (modernc.org/sqlite, file singolo)
```

I package "puri" (`macros`, `planner`, `tdee`, `domain`) non hanno side-effect e
sono testati in isolamento; i `service` li orchestrano con i repository DB.

---

## Package interni (`internal/`)

| Package | Responsabilità |
|---------|----------------|
| `app/` | Router chi, middleware, handler HTTP, rate limiter, validazione |
| `auth/` | Hashing password (bcrypt cost 12), helper sessioni |
| `config/` | Caricamento variabili d'ambiente `NUTREEK_*` |
| `db/` | Repository per ogni dominio; `db/sqlc/` è generato, `db/migrations/` e `db/queries/` sono sorgenti |
| `domain/` | Struct di dominio pure (nessuna business logic) |
| `macros/` | Calcoli macro (Kcal/Proteine/Carbo/Grassi) puri e testati |
| `off/` | Client OpenFoodFacts, sync del dump locale, reconciler periodico |
| `planner/` | Algoritmo greedy di generazione del piano settimanale |
| `profile/` | Calcolo profilo utente (BMI, BMR, target macro) |
| `recipeimport/` | Import ricetta da URL (`schema.org/Recipe`, JSON-LD/microdata) |
| `service/` | Orchestrazione: `PlannerService`, `TDEEService` |
| `tdee/` | Stima TDEE adattiva (algoritmo puro su trend peso/intake) |
| `web/` | Template HTML + asset statici, embeddati nel binary |

---

## Flussi chiave

### Autenticazione e sessioni
Registrazione/login in `internal/app/auth_handlers.go`; password con bcrypt
(`internal/auth`). Le sessioni usano SCS v2 con store SQLite (tabella `sessions`).
Il middleware in `internal/app/middleware.go` carica la sessione, applica CSRF
(nosurf) e popola l'utente nel context. Ogni risorsa è scoped all'utente
autenticato.

### Feature gating premium
Cuore del modello di monetizzazione. `HasFeature(ctx, user, key)` in
`internal/app/features.go` decide l'accesso:
- `NUTREEK_PREMIUM_ENABLED=false` → tutto accessibile (default);
- owner (`NUTREEK_OWNER_EMAIL`) → accesso totale;
- feature disabilitata (kill-switch) → bloccata per tutti;
- feature non-premium → accessibile;
- feature premium → richiede sottoscrizione attiva (`internal/db/subscriptions.go`).

Il middleware `requireFeature(key)` protegge le route (HTML→redirect `/upgrade`,
HTMX→`HX-Redirect`, API→`403 JSON`). Le definizioni feature stanno in
`internal/db/features.go`; una cache in-memory (`sync.RWMutex`, TTL 60s) evita
query ripetute ed è invalidata ad ogni toggle dall'admin panel.

### Generazione automatica del piano
`internal/planner/generator.go` implementa un algoritmo greedy che riempie la
settimana pescando dalle varianti del piano nutrizionista per avvicinarsi ai
target macro dell'utente. Orchestrato da `internal/service/planner_service.go`.

### TDEE adattivo
`internal/tdee/estimator.go` stima il fabbisogno calorico reale dal trend
settimanale peso-vs-intake (dati da `weight_logs` + storico pasti).
`internal/service/tdee_service.go` espone il calcolo e il suggerimento di
aggiornamento target.

### OpenFoodFacts (ricerca locale)
`internal/off/dump.go` scarica settimanalmente il dump pubblico OFF, lo
stream-parsa filtrando i prodotti italiani e popola `off_products` +
indice FTS5 `off_products_fts`. Le ricerche sono 100% locali (<50ms), zero
chiamate HTTP nel search-path. Il lookup per barcode resta live verso l'API OFF
(`internal/off/client.go`), usato solo al primo import di un prodotto nuovo.
Dettagli e motivazione in [CLAUDE.md](../CLAUDE.md#strategia-openfoodfacts-off).

### REST API v1
`internal/app/api_handlers.go` espone endpoint JSON sotto `/api/v1/*`,
autenticati con Bearer token. I token sono generati come `nutreek_<hex>` e
memorizzati come hash SHA-256 (`internal/db/api_tokens.go`). Il gruppo route è
protetto da `requireAPIToken` + `requireFeature("api_access")`.

### Coach mode (RBAC)
Gli utenti con ruolo `coach` accedono al gruppo `/coach/*`
(`internal/app/coach_handlers.go`). Possono gestire una libreria di piani
nutrizionali e assegnarli ai clienti collegati (`internal/db/coach.go`,
tabella `coach_clients`). Protetto da `requireCoach` + `requireFeature("coach_mode")`.

### Admin panel
`internal/app/admin_panel_handlers.go` espone `/admin/panel`, accessibile solo
all'owner (email in `NUTREEK_OWNER_EMAIL`). Gestisce feature flags, lista utenti
e assegnazione sottoscrizioni premium.

---

## Schema dati (alto livello)

Tabelle principali raggruppate per dominio (dettaglio completo in
`internal/db/migrations/`):

- **Utenti & sessioni**: `users` (con profilo, ruolo, target macro), `sessions`
- **Piani & pasti**: `weekly_plans`, `meals`, `meal_items`,
  `meal_item_customizations`, `meal_cheat_items`, `meal_templates`,
  `meal_template_slots`
- **Piano nutrizionista**: `nutritionist_plans`, `nutritionist_meal_types`,
  `nutritionist_meal_variations`, `nutritionist_meal_items`
- **Ingredienti & OFF**: `ingredients`, `ingredient_categories`, `allergen_tags`,
  `ingredient_allergens`, `off_products`, `off_products_fts` (FTS5), `off_sync_state`
- **Ricette**: `recipes`, `recipe_ingredients`
- **Tracking**: `weight_logs`, `water_logs`, `workout_activities`, `workout_logs`
- **Lista spesa**: `shopping_lists`, `shopping_list_items`, `shopping_user_items`
- **Coach**: `coach_clients`
- **Monetizzazione**: `features`, `subscriptions`, `api_tokens`

Le migrazioni Goose sono embeddate ed eseguite automaticamente all'avvio.

---

## Decisioni di design notevoli

- **SQLite pure-Go (no CGO)**: `modernc.org/sqlite`. Build statica
  (`CGO_ENABLED=0`), cross-compile banale, immagine Docker distroless ~20 MB.
- **OFF locale invece che runtime**: gli endpoint pubblici OFF non sono
  affidabili per search-as-you-type (rate limit, latenza, ~20% 503). Eliminare
  la dipendenza runtime è l'unica via 100% affidabile.
- **Embedding di template/CSS/migrazioni**: un singolo binario senza file esterni
  da deployare; il CSS Tailwind è generato a compile-time e va rigenerato dopo
  modifiche.
- **Cache feature 60s**: il gating premium è sul path di ogni richiesta; la cache
  evita query DB ripetute mantenendo reattività ai toggle admin (invalidazione).
- **Server-side + HTMX**: nessun build frontend, nessuna SPA da mantenere;
  interattività via HTMX e piccoli script vanilla.

---

## Stato test e qualità

- `go test ./...` verde. Copertura su: auth, macros, db layer (tutti i repo),
  OFF client/reconciler, rate limiter, API tokens, features, planner, tdee.
- Pattern: tabelle di test, `t.Cleanup()`, SQLite temporaneo per isolamento.
- **Non presente**: test end-to-end/UI, pipeline CI/CD, linter configurato.
  Sono le aree naturali di investimento per un acquirente che voglia
  industrializzare il prodotto.
