# Handover — Nutreek

Checklist e note operative per il passaggio di consegne a un nuovo proprietario
del codice. Per l'architettura vedi [ARCHITECTURE.md](ARCHITECTURE.md); per la
configurazione completa vedi [README.md](../README.md).

---

## Cosa viene trasferito

- **Repository Git completo** con l'intera storia dei commit.
- **Tutto il codice sorgente**: backend Go, template HTML/HTMX, sorgenti CSS
  Tailwind, migrazioni SQL, query SQL scritte a mano nei repo.
- **Documentazione**: README, CLAUDE.md (memoria di progetto), ROADMAP.md,
  CHANGELOG.md, questo asset pack, screenshot in `docs/screenshots/`.
- **Tutti i diritti** sul codice e sul nome del progetto, secondo i termini
  dell'accordo di vendita. Il codice è pubblicato con licenza AGPL-3.0 (vedi
  `LICENSE`); chi detiene i diritti può concedere licenze diverse.

## Cosa NON è incluso (da predisporre a cura dell'acquirente)

- Dominio, hosting, certificati TLS / reverse proxy.
- Eventuale registrazione del marchio (non registrato).
- Base utenti, dati di produzione, ricavi (il prodotto è pre-revenue).
- Account o chiavi verso servizi terzi (nessuno è richiesto: OpenFoodFacts è
  pubblico e non necessita di credenziali).

---

## Prerequisiti

- **Go 1.27+** (https://go.dev/dl/) — unico requisito per lo sviluppo nativo.
- (Opzionale) **Docker 24+ / Compose v2** per il deploy in container.
- Il CLI standalone di Tailwind viene scaricato automaticamente in `./bin/`.

## Build & avvio

```bash
make setup       # scarica Tailwind CLI in ./bin/
make generate    # rigenera il CSS Tailwind
make dev         # go run ./cmd/nutreek -> http://localhost:8080
make build       # binario statico (CGO_ENABLED=0)
make test        # go test ./...
make docker-up   # docker compose up --build
```

Il DB SQLite viene creato in `./data/nutreek.db` al primo avvio; le migrazioni
sono applicate automaticamente. Il CSS generato è embeddato nel binario a
compile-time: **rigenerarlo** (`make tailwind`) dopo modifiche a template o
classi prima di `go build`.

---

## Configurazione

Tutte le variabili `NUTREEK_*` sono documentate nella tabella in
[README.md](../README.md#configurazione). Le più rilevanti per il deploy:

| Variabile | Quando |
|-----------|--------|
| `NUTREEK_SESSION_KEY` | **Obbligatoria in `prod`** — generare con `openssl rand -hex 32` |
| `NUTREEK_ENV=prod` | Abilita secure cookies |
| `NUTREEK_OWNER_EMAIL` | Email che ottiene accesso a `/admin/panel` |
| `NUTREEK_PREMIUM_ENABLED` | `true` per attivare il gating premium |
| `NUTREEK_DB_PATH` | Percorso del file SQLite (persistere su volume) |

---

## Punti di attenzione operativi

- **Primo avvio OFF**: la tabella `off_products` parte vuota; lo scheduler lancia
  un sync iniziale in background (~0.9 GB di download, ~10 min). Le ricerche
  ingredienti restano vuote finché il sync non completa. Disattivabile con
  `NUTREEK_OFF_SYNC_ENABLED=false`.
- **Backup**: il DB è un singolo file. Backup automatico via
  `NUTREEK_BACKUP_ENABLED=true`; oppure a caldo con
  `sqlite3 ./data/nutreek.db ".backup ./data/nutreek.backup.db"`.
- **TLS**: il container non termina TLS — usare sempre un reverse proxy davanti
  in produzione pubblica (esempio Caddy nel README).
- **Healthcheck**: `nutreek -healthcheck` (probe HTTP locale su `/healthz`), già
  configurato in `docker-compose.yml`.
- **Dati sanitari**: peso/intake sono dati personali; valutare gli obblighi
  GDPR applicabili nel proprio contesto di deploy. Esistono già export dati
  utente (Art. 20) e cancellazione account (Art. 17).

---

## Primi passi consigliati per il nuovo proprietario

1. Clonare il repo, `make setup && make generate && make test` — verde atteso.
2. `make dev`, registrare un utente con l'email impostata in
   `NUTREEK_OWNER_EMAIL` e visitare `/admin/panel`.
3. Leggere [ARCHITECTURE.md](ARCHITECTURE.md) per la mappa del codice.
4. Aree di investimento naturali (oggi assenti): CI/CD, linter, test E2E,
   eventuale PWA/scanner barcode (già predisposto lato backend — vedi ROADMAP).
