# Changelog

Tutte le modifiche notevoli a questo progetto sono documentate in questo file.

Il formato segue [Keep a Changelog](https://keepachangelog.com/it/1.1.0/).

---

## [Non rilasciato]

### Aggiunto
- Immagine Docker privata su GHCR pubblicata a ogni release (`:latest`, `:vX.Y.Z`, `:vX.Y`), flag `nutreek -version` (#111)

### Cambiato
- Restyle «Sereno», prima parte (#121): nuovo sistema di temi (modalità chiaro / scuro / come il sistema × colore bosco / mare / tramonto / grafite, in cookie e letto dal server), font Figtree servito dall'app, sidebar su desktop e barra in basso su telefono con il foglio «Altro», menu «Aspetto». La settimana mostra un giorno alla volta: schede dei giorni, riepilogo con anello kcal e macro, pasti, acqua e allenamenti; il giorno aperto è nell'URL (`?day=`). I vecchi cookie `theme` vengono convertiti.

### Manutenzione
- Go 1.27.2; chi 5.5, goose 3.29, modernc sqlite 1.60, x/crypto 0.58; SortableJS 1.15.7; GitHub Actions alle major correnti, golangci-lint fissato a v2.14.0; immagini base `golang:1.27-alpine` e `alpine:3.24`; `.gitattributes` con fine riga LF (#112)

### In corso
- Premium gating effettivo (#70): `requireFeature("api_access")` su `/api/v1/*`, badge Premium nell'UI, `Features map[string]bool` in `authPageData`
- README aggiornato con funzionalità Round 5, env vars, esempi REST API (#69)
- Documentazione aggiornata: CLAUDE.md, ROADMAP.md, CHANGELOG.md (#68)

---

## [Round 5] — 2026-06-12

### Aggiunto
- **Coach mode** (#63): libreria piani nutrizionista, assegnazione a clienti, storico freeze piano, dashboard coach con aderenza per cliente
- **REST API v1** (#65): autenticazione Bearer token, endpoint `GET /me`, `GET /week`, `GET /ingredients`, `GET /activities`, `POST /water`, `POST /weight`, `POST /workout`; gestione token in `/settings`
- **Admin panel** (#66): pannello owner-only `/admin/panel` con gestione feature flags (abilitazione/disabilitazione/premium), elenco utenti, assegnazione sottoscrizioni premium
- **Feature flag system**: tabella `features` con kill-switch + is_premium, middleware `requireFeature(key)`, helper `HasFeature(ctx, user, key)`, cache TTL 60s
- **Subscriptions**: tabella `subscriptions` con date inizio/fine, metodo `IsActive`, gestione da admin panel
- **API tokens**: tabella `api_tokens` (SHA-256), creazione/revoca da impostazioni, middleware `requireAPIToken`, `LastUsedAt` aggiornato ad ogni uso

### Tecnico
- Migrazioni 0023 (coach), 0024 (subscriptions/features), 0025 (api_tokens)
- Nuovi package: `internal/db` — `coach.go`, `api_tokens.go`, `features.go`, `subscriptions.go`
- CSRF exempt su `/api/*` e `/admin/*`
- Risposta JSON strutturata per tutti gli endpoint API

---

## [Round 4] — 2026

### Aggiunto
- **Generazione automatica piano** (#62): algoritmo greedy che riempie la settimana pescando dalle varianti nutrizionista, puntando ai target macro
- **Adherence analytics** (#64): score aderenza giornaliero/settimanale, streak rispetto piano, compliance % per tipo pasto, grafici nella pagina statistiche

### Tecnico
- Nuovi package: `internal/planner/`, `internal/service/planner.go`
- Form "Genera settimana" nella dashboard (visibile solo a settimana vuota con piano attivo)

---

## [Round 3] — 2026

### Aggiunto
- **TDEE adattivo** (#61): stima TDEE reale dal trend peso/intake delle ultime settimane, suggerimento automatico aggiustamento target macro, pulsante "Accetta" nella pagina statistiche
- **Micronutrienti** (#60): fibre, zuccheri, grassi saturi, sodio estratti dal dump OFF; visualizzazione nel dettaglio ingrediente e ricetta

### Tecnico
- Nuovi package: `internal/tdee/`, `internal/service/tdee.go`
- Nuovi campi su `ingredients`: `fiber_g`, `sugar_g`, `saturated_fat_g`, `sodium_mg`

---

## [Round 2] — 2026

### Aggiunto
- **Tracciamento idratazione** (#58): log giornaliero `water_logs`, widget nella day card, target ml nel profilo, sparkline in statistiche
- **Import ricetta da URL** (#57): parse `schema.org/Recipe` (JSON-LD e microdata), pre-compilazione automatica form ricetta
- **Filtro allergeni** (#56): tag allergeni sulla scheda ingrediente, filtro multi-allergene nella lista ingredienti, warning inline nel piano e nei picker

### Tecnico
- Nuovo package: `internal/recipeimport/`
- Migrazione tabella `water_logs`
- 14 allergeni pre-definiti (spec EU)

---

## [Round 1] — 2026

### Aggiunto
- **Ricerca OFF locale FTS5**: sync settimanale dump OFF (~250k prodotti italiani), ricerche <50ms, zero dipendenze runtime dalla rete
- **Profilo utente**: peso/altezza, BMI/BMR calcolati, target macro personalizzati
- **Allenamenti**: log allenamenti con calorie bruciate, dashboard settimanale attività
- **Temi UI**: forest/ocean/sunset/mono, persistenza via cookie
- **Sidebar PIN**: navigazione laterale collassabile, stato PIN persistito
- **Tour guidato**: onboarding modale per nuovi utenti
- **Statistiche grafici**: grafici peso, idratazione, macro settimanali con chart.js
- **Piano nutrizionista**: struttura gerarchica meal types → varianti → items, assegnazione utente

### Tecnico
- Package `internal/off/dump.go`: Syncer con stream-parse JSONL.gz
- Migrazioni 0001–0022
- Rate limiting per IP sul login
- Security headers (HSTS, CSP, X-Frame-Options)
- Export/import CSV piani settimanali
- Export dati utente JSON (GDPR Art. 20)
- Cancellazione account (GDPR Art. 17)
