# Handover — Nutreek

Checklist and operational notes for handing the code over to a new owner. For
the architecture see [ARCHITECTURE.md](ARCHITECTURE.md); for the full
configuration see [README.md](../README.md).

---

## What is transferred

- **The complete Git repository**, with its full commit history.
- **All the source code**: Go backend, HTML/HTMX templates, Tailwind CSS
  sources, SQL migrations, the hand-written SQL in the repositories.
- **Documentation**: README, CLAUDE.md (project memory), ROADMAP.md,
  CHANGELOG.md, this asset pack, the screenshots in `docs/screenshots/`.
- **All rights** to the code and the project name, under the terms of the sale
  agreement. The code is published under the AGPL-3.0 license (see `LICENSE`);
  whoever holds the rights can grant other licenses.

## What is NOT included (for the buyer to set up)

- Domain, hosting, TLS certificates / reverse proxy.
- A trademark registration (the name is not registered).
- A user base, production data, revenue (the product is pre-revenue).
- Accounts or keys for third-party services (none is needed: OpenFoodFacts is
  public and needs no credentials).

---

## Prerequisites

- **Go 1.27+** (https://go.dev/dl/) — the only requirement for native development.
- (Optional) **Docker 24+ / Compose v2** for container deployment.
- The standalone Tailwind CLI is downloaded automatically into `./bin/`.

## Build & run

```bash
make setup       # download the Tailwind CLI into ./bin/
make generate    # regenerate the Tailwind CSS
make dev         # go run ./cmd/nutreek -> http://localhost:8080
make build       # static binary (CGO_ENABLED=0)
make test        # go test ./...
make docker-up   # docker compose up --build
```

The SQLite DB is created in `./data/nutreek.db` on first start; migrations are
applied automatically. The generated CSS is embedded in the binary at build
time: **regenerate it** (`make tailwind`) after changing templates or classes,
before `go build`.

---

## Configuration

Every `NUTREEK_*` variable is documented in the table in
[README.md](../README.md#configuration). The most relevant for a deployment:

| Variable | When |
|-----------|--------|
| `NUTREEK_SESSION_KEY` | **Required in `prod`** — generate it with `openssl rand -hex 32` |
| `NUTREEK_ENV=prod` | Enables secure cookies |
| `NUTREEK_OWNER_EMAIL` | The email that gets access to `/admin/panel` |
| `NUTREEK_PREMIUM_ENABLED` | `true` to turn on premium gating |
| `NUTREEK_DB_PATH` | Path of the SQLite file (keep it on a volume) |

---

## Operational notes

- **First OFF start**: the `off_products` table starts empty; the scheduler runs
  an initial sync in the background (~0.8 GB download, ~10 min). Ingredient
  searches return nothing until the sync completes. It can be turned off with
  `NUTREEK_OFF_SYNC_ENABLED=false`.
- **Backup**: the DB is a single file. Automatic backups with
  `NUTREEK_BACKUP_ENABLED=true`, or a hot copy with
  `sqlite3 ./data/nutreek.db ".backup ./data/nutreek.backup.db"`.
- **TLS**: the container does not terminate TLS — always put a reverse proxy in
  front of a public deployment (a Caddy example is in the README).
- **Healthcheck**: `nutreek -healthcheck` (a local HTTP probe on `/healthz`),
  already configured in `docker-compose.yml`.
- **Health data**: weight and intake are personal data; assess the GDPR
  obligations that apply to your deployment. User data export (Art. 20) and
  account deletion (Art. 17) already exist.

---

## Suggested first steps for the new owner

1. Clone the repo, `make setup && make generate && make test` — expect green.
2. `make dev`, register a user with the email set in `NUTREEK_OWNER_EMAIL` and
   open `/admin/panel`.
3. Read [ARCHITECTURE.md](ARCHITECTURE.md) for a map of the code.
4. A natural area of investment, missing today: end-to-end tests.
