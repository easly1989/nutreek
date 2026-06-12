# Nutreek — Roadmap

> Posizionamento: **"Il sistema operativo nutrizionale self-hosted"** — l'unico strumento privacy-first che copre sia il tracking personale adattivo sia il workflow nutrizionista↔cliente, senza cloud, senza abbonamenti.

---

## Stato di avanzamento

| Sprint | Stato | Note |
|--------|-------|------|
| Sprint 1 — Quick win | ✅ Completato | Round 1–3 |
| Sprint 2 — Differenzianti | ✅ Completato | Round 3–4 |
| Sprint 3 — Ambiziosi | ✅ Completato | Round 4–5 |
| Monetizzazione | ✅ Completato | Round 5 |
| Round 6 — Documentazione & Gating | 🔄 In corso | Issue #68, #69, #70 |

---

## Asset differenzianti esistenti

| Asset | Vantaggio vs concorrenti |
|---|---|
| Ricerca OFF locale FTS5 (<50ms, no rate limit) | Più veloce e affidabile di MyFitnessPal/Cronometer |
| Tracking sgarri con override macro effettivi | Quasi unico: distingue "piano" da "mangiato davvero" |
| Piani nutrizionista gerarchici (meal types → varianti → items) | Niche non servito nel self-hosted |
| Template settimanali + lista spesa multi-settimana | In linea con Mealime/Paprika |
| TDEE adattivo dal trend peso/intake | MacroFactor vende questa feature a $12/mese |
| Coach mode: libreria piani, assegnazione clienti, storico freeze | Non servito in nessuna app self-hosted |
| REST API v1 con Bearer token | Integrazione con Home Assistant, n8n, script personali |
| Self-hosted, ~20 MB binario, zero cloud, zero CGO | Moat strutturale vs tutto il mercato cloud |

---

## Sprint 1 — Quick win ✅

### `#1` Filtro allergeni nell'UI — ✅ [#56](https://github.com/easly1989/nutreek/issues/56)
Tag allergeni sulla scheda ingrediente, filtro nella lista, warning inline nel piano e nei picker.

### `#2` Import ricetta da URL — ✅ [#57](https://github.com/easly1989/nutreek/issues/57)
Incolla un link a qualsiasi sito di ricette: parse del microdata `schema.org/Recipe` (JSON-LD o microdata), pre-compila nome, ingredienti, porzioni.

### `#3` Idratazione — ✅ [#58](https://github.com/easly1989/nutreek/issues/58)
Tabella `water_logs`, widget nella day card, target giornaliero nel profilo, sparkline negli stats.

---

## Sprint 2 — Differenzianti ✅

### `#4` PWA + scanner barcode con fotocamera — ⏸ Deferred [#59](https://github.com/easly1989/nutreek/issues/59)
Service worker + web manifest + BarcodeDetector. Rimandato a fase successiva.

### `#5` Micronutrienti chiave — ✅ [#60](https://github.com/easly1989/nutreek/issues/60)
Fibre, zuccheri, grassi saturi, sodio dal dump OFF. Esteso `ingredients`, `macros.go` e UI.

### `#6` ⭐ TDEE adattivo (stile MacroFactor) — ✅ [#61](https://github.com/easly1989/nutreek/issues/61)
Stima TDEE reale dal trend settimanale peso-vs-intake, suggerimento automatico aggiustamento target macro.

---

## Sprint 3 — Ambiziosi ✅

### `#7` Generazione automatica del piano — ✅ [#62](https://github.com/easly1989/nutreek/issues/62)
Algoritmo greedy: dati i target + pool varianti nutrizionista, riempie la settimana centrando i macro.

### `#8` Coach mode multi-cliente — ✅ [#63](https://github.com/easly1989/nutreek/issues/63)
Libreria piani, assegnazione clienti, storico freeze, dashboard coach con aderenza. RBAC implementato.

### `#9` Adherence analytics (piano vs reale) — ✅ [#64](https://github.com/easly1989/nutreek/issues/64)
Score aderenza giornaliero/settimanale, streak rispetto piano, compliance % per tipo pasto.

### `#10` REST API + automazioni — ✅ [#65](https://github.com/easly1989/nutreek/issues/65)
Bearer token API v1: `GET /me`, `GET /week`, `GET /ingredients`, `GET /activities`, `POST /water`, `POST /weight`, `POST /workout`.

---

## Monetizzazione ✅

### `#11` Pannello admin + feature flag premium con sottoscrizioni — ✅ [#66](https://github.com/easly1989/nutreek/issues/66)
Pannello web owner-only (`/admin/panel`). Feature flags con kill-switch + is_premium. Sottoscrizioni per-utente. Middleware `requireFeature`. Helper `HasFeature`. Cache 60s con invalidazione.

Feature premium: `auto_plan`, `tdee_adaptive`, `coach_mode`, `api_access`.

---

## Round 6 — Documentazione & Gating effettivo 🔄

### `#68` Documentazione aggiornata — 🔄 In corso
CLAUDE.md (nuove env vars, nuovi package, sezione premium), ROADMAP.md (questo file), CHANGELOG.md.

### `#69` README con screenshot — 🔄 In corso
README aggiornato con tabella funzionalità completa, env vars, esempi REST API, sezione Admin & Premium.

### `#70` Premium gating effettivo — ✅ Implementato
- `requireFeature("api_access")` aggiunto al gruppo `/api/v1/*`
- `POST /settings/api-tokens` gated con `requireFeature("api_access")`
- `Features map[string]bool` in `authPageData`, popolato in `baseData()` per ogni richiesta
- Badge "Premium" nei template (dashboard, stats, settings)
- Risposta JSON 403 dal middleware `requireFeature` per le route API

---

## Matrice impatto / sforzo

```
Alto impatto
     │
  #6 │         #8   #11
  #4 │    #7       #10
  #2 │  #5   #9
  #1 │        #3
     └─────────────────── Alto sforzo
      basso         alto
```

Ordine di esecuzione seguito: `#1` → `#2` → `#3` → `#5` → `#6` → `#9` → `#7` → `#8` → `#11` → `#10` → `#68/69/70`

---

## Note tecniche generali

- Stack bloccato: Go 1.25+, chi, SQLite (modernc), HTMX, Tailwind v3.4.17
- Nessuna dipendenza cloud obbligatoria per nessuna feature
- Ogni feature su branch dedicato `feat/<scope>-<descrizione>` con conventional commits in italiano
- `go build ./...` + `go test ./...` devono essere verdi prima di ogni push

---

## Issue GitHub (tutte le feature)

| # | Titolo | Sprint | Stato |
|---|---|---|---|
| [#56](https://github.com/easly1989/nutreek/issues/56) | Filtro allergeni nell'UI | Sprint 1 | ✅ Chiusa |
| [#57](https://github.com/easly1989/nutreek/issues/57) | Import ricetta da URL (schema.org/Recipe) | Sprint 1 | ✅ Chiusa |
| [#58](https://github.com/easly1989/nutreek/issues/58) | Tracciamento idratazione | Sprint 1 | ✅ Chiusa |
| [#59](https://github.com/easly1989/nutreek/issues/59) | PWA installabile + scanner barcode | Sprint 2 | ⏸ Deferred |
| [#60](https://github.com/easly1989/nutreek/issues/60) | Micronutrienti chiave | Sprint 2 | ✅ Chiusa |
| [#61](https://github.com/easly1989/nutreek/issues/61) | ⭐ TDEE adattivo (flagship) | Sprint 2 | ✅ Chiusa |
| [#62](https://github.com/easly1989/nutreek/issues/62) | Generazione automatica del piano | Sprint 3 | ✅ Chiusa |
| [#63](https://github.com/easly1989/nutreek/issues/63) | Coach mode multi-cliente (RBAC) | Sprint 3 | ✅ Chiusa |
| [#64](https://github.com/easly1989/nutreek/issues/64) | Analytics aderenza + streak | Sprint 3 | ✅ Chiusa |
| [#65](https://github.com/easly1989/nutreek/issues/65) | REST API + integrazioni self-hosted | Sprint 3 | ✅ Chiusa |
| [#66](https://github.com/easly1989/nutreek/issues/66) | Pannello admin + feature flag + sottoscrizioni | Monetizzazione | ✅ Chiusa |
| [#67](https://github.com/easly1989/nutreek/issues/67) | Roadmap tracking Round 5 | Tracking | ✅ Chiusa |
| [#68](https://github.com/easly1989/nutreek/issues/68) | Documentazione aggiornata (CLAUDE.md, ROADMAP, CHANGELOG) | Round 6 | 🔄 In corso |
| [#69](https://github.com/easly1989/nutreek/issues/69) | README con screenshot | Round 6 | 🔄 In corso |
| [#70](https://github.com/easly1989/nutreek/issues/70) | Premium gating effettivo | Round 6 | ✅ Implementato |
