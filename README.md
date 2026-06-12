# Nutreek

Webapp self-hosted per gestire il piano nutrizionale settimanale: ingredienti condivisi (con integrazione OpenFoodFacts), ricette, template di pasti, vista settimanale con calcolo macro (Kcal / Proteine / Carboidrati / Grassi) per pasto, giorno e settimana.

Eseguibile sia **nativamente** (binario Go, nessuna dipendenza esterna) sia in un **singolo container Docker** (nessun servizio esterno: SQLite embedded).

> Stato: MVP in sviluppo. Il modulo allenamenti arriverà in una fase successiva.

---

## Stack

- **Backend:** Go 1.25+ (chi, scs, sqlc, goose, bcrypt, nosurf)
- **DB:** SQLite via `modernc.org/sqlite` (pure Go, no CGO)
- **Frontend:** server-side rendering con `html/template` + HTMX + Tailwind CSS
- **Deploy:** binario statico o immagine Docker distroless (~20 MB)

## Requisiti

Per sviluppo nativo (senza Docker):
- Go 1.25 o superiore — https://go.dev/dl/
- Nient'altro. Il CLI standalone di Tailwind viene scaricato automaticamente in `./bin/`.

Per deploy in container:
- Docker 24+ e Docker Compose v2

## Quickstart (dev, senza Docker)

### Linux / macOS (con `make`)

```bash
make setup       # scarica Tailwind CLI in ./bin/
make tailwind    # genera internal/web/static/css/tailwind.css
make dev         # go run ./cmd/nutreek -> http://localhost:8080
```

### Windows (PowerShell, senza `make`)

```powershell
pwsh scripts/setup.ps1       # scarica Tailwind CLI in .\bin\
pwsh scripts/tailwind.ps1    # build CSS (aggiungere -Watch per watch mode)
pwsh scripts/dev.ps1         # avvia il server su http://localhost:8080
```

In alternativa, se hai installato GNU Make su Windows (es. `winget install ezwinports.make`) funzionano anche i target del `Makefile`.

Il DB SQLite viene creato in `./data/nutreek.db` al primo avvio e le migrazioni sono applicate automaticamente. Il CSS generato (`internal/web/static/css/tailwind.css`) è embeddato nel binario a compile-time: va **sempre** rigenerato prima di `go build` o `go run` dopo aver modificato template o classi Tailwind.

## Quickstart (Docker)

```bash
# Genera una session key sicura la prima volta
export NUTREEK_SESSION_KEY=$(openssl rand -hex 32)

docker compose up --build
# -> http://localhost:8080
```

Il DB persiste su `./data/` (mappato su `/data` nel container).

## Configurazione

Variabili d'ambiente principali:

| Variabile | Default | Descrizione |
|---|---|---|
| `NUTREEK_ADDR` | `:8080` | Indirizzo di ascolto HTTP |
| `NUTREEK_DB_PATH` | `./data/nutreek.db` | Path del file SQLite |
| `NUTREEK_ENV` | `dev` | `dev` o `prod` (in `prod` abilita secure cookies) |
| `NUTREEK_SESSION_KEY` | *(auto in dev)* | Chiave HMAC per i cookie; **obbligatoria in `prod`** |
| `NUTREEK_OFF_BASE_URL` | `https://world.openfoodfacts.org` | Override API OpenFoodFacts |
| `NUTREEK_RECONCILE_INTERVAL` | `24h` | Frequenza job di riconciliazione ingredienti locali vs OFF |
| `NUTREEK_RECONCILE_ENABLED` | `true` | Abilita/disabilita il job |
| `NUTREEK_ADMIN_TOKEN` | *(opzionale)* | Token per endpoint amministrativi |

## Deploy pubblico (dietro reverse proxy)

Esempio minimale con [Caddy](https://caddyserver.com/):

```caddyfile
nutreek.example.com {
    reverse_proxy nutreek:8080
}
```

Il container **non** termina TLS: assumi sempre un reverse proxy davanti in produzione pubblica.

L'immagine usa `gcr.io/distroless/static:nonroot` come base finale (~15 MB totali). Niente shell, niente pacchetti: il binario e' compilato in pure Go (`CGO_ENABLED=0`, SQLite via `modernc.org/sqlite`). Il container espone una healthcheck via sub-comando `nutreek -healthcheck` (HTTP probe locale su `/healthz`), gia' configurato in `docker-compose.yml` con interval 30s.

Sub-comandi del binario:

```bash
nutreek                # avvia il server
nutreek -healthcheck   # GET /healthz, exit 0 se ok 1 altrimenti
```

Endpoint amministrativi (se `NUTREEK_ADMIN_TOKEN` e' settato):

```bash
# Forza una riconciliazione on-demand (matcha local → OFF, idempotente).
curl -H "X-Admin-Token: $NUTREEK_ADMIN_TOKEN" -X POST \
     http://localhost:8080/admin/reconcile-ingredients
# -> {"considered":N,"matched":...,"no_match":...,"errors":...}
```

## Backup

Il DB è un singolo file SQLite. Backup consigliato:

```bash
# A caldo (sqlite3 CLI)
sqlite3 ./data/nutreek.db ".backup ./data/nutreek.backup.db"
```

## Struttura del repository

```
cmd/nutreek/          entrypoint
internal/
  app/                router, middleware, handler HTTP
  auth/               password hashing, sessioni
  db/
    migrations/       SQL versionate (embedded)
    queries/          SQL input per sqlc
    sqlc/             codice generato
  domain/             struct di dominio pure
  macros/             calcolo macro (puro, testabile)
  off/                client OpenFoodFacts + reconciliation
  service/            business logic
  web/
    templates/        pagine e partial HTMX (embedded)
    static/           css, js (embedded, generati)
web/tailwind/         input.css e tailwind.config.js (sorgenti)
scripts/              setup.ps1, tailwind.ps1, dev.ps1 (Windows)
```

## Licenza

**Proprietaria — All Rights Reserved.** Vedi [LICENSE](LICENSE).

Il codice è visibile ma **non** è open source: nessun uso, copia o distribuzione è consentito senza autorizzazione scritta del titolare.
