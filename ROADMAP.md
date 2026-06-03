# Nutreek — Roadmap

> Posizionamento: **"Il sistema operativo nutrizionale self-hosted"** — l'unico strumento privacy-first che copre sia il tracking personale adattivo sia il workflow nutrizionista↔cliente, senza cloud, senza abbonamenti.

---

## Asset differenzianti esistenti

| Asset | Vantaggio vs concorrenti |
|---|---|
| Ricerca OFF locale FTS5 (<50ms, no rate limit) | Più veloce e affidabile di MyFitnessPal/Cronometer |
| Tracking sgarri con override macro effettivi | Quasi unico: distingue "piano" da "mangiato davvero" |
| Piani nutrizionista gerarchici (meal types → varianti → items) | Niche non servito nel self-hosted |
| Template settimanali + lista spesa multi-settimana | In linea con Mealime/Paprika |
| Self-hosted, ~20 MB binario, zero cloud, zero CGO | Moat strutturale vs tutto il mercato cloud |

---

## Sprint 1 — Quick win (valore immediato, basso sforzo)

### `#1` Filtro allergeni nell'UI
Lo schema è già completo (`allergen_tags`, `ingredient_allergens`, 14 allergeni pre-definiti).
Manca solo l'interfaccia: tag sulla scheda ingrediente, filtro nella lista, warning inline nel piano e nei picker.

### `#2` Import ricetta da URL
Incolla un link a qualsiasi sito di ricette: Nutreek fa GET + parse del microdata `schema.org/Recipe` (JSON-LD o microdata).
Pre-compila nome, ingredienti con quantità, porzioni. Concorrente diretto di Paprika/Tandoor.

### `#3` Idratazione
Tabella `water_logs`, widget nella day card, target giornaliero nel profilo, sparkline negli stats.
Standard atteso nella categoria, costo di implementazione minimo.

---

## Sprint 2 — Differenzianti medi

### `#4` PWA + scanner barcode con fotocamera
Service worker + web manifest = installabile da mobile. `BarcodeDetector` (o html5-qrcode) chiama il backend `GetByBarcode` **già esistente**. Trasforma Nutreek in app usabile al supermercato.

### `#5` Micronutrienti chiave
Il dump OFF contiene già fibre, zuccheri, grassi saturi, sodio. Estendi `ingredients`, `macros.go` e l'UI. Differenziante verso i tracker basici senza sforzo di data collection.

### `#6` ⭐ TDEE adattivo (stile MacroFactor)
Usa `weight_logs` + storico macro pasti + `workout_logs` (tutti già presenti) per stimare il fabbisogno calorico reale dal trend settimanale peso-vs-intake e suggerire aggiustamenti ai target.
**Nessuna app self-hosted ce l'ha.** MacroFactor vende questa feature a $12/mese.

---

## Sprint 3 — Ambiziosi / cambiano categoria

### `#7` Generazione automatica del piano
Dati i target + pool di ricette/varianti nutrizionista: riempi giorno/settimana per centrare i macro (algoritmo greedy o solver). Le varianti nutrizionista sono il substrato perfetto.

### `#8` Coach mode multi-cliente
Account "nutrizionista" che assegna `nutritionist_plans` a clienti, vede l'aderenza e gli sgarri in un pannello. Apre il mercato *pro* self-hosted (oggi non servito da nessuno). Richiede RBAC.

### `#9` Adherence analytics (piano vs reale)
Nutreek è l'unico a tracciare sia il piano sia gli sgarri: combinali in uno score di aderenza giornaliero/settimanale, streak di rispetto del piano, "compliance %" per tipo di pasto.

### `#10` REST API + automazioni
Token API read/write per l'ecosistema self-hosted (Home Assistant, n8n, script personali). Il pubblico self-hosted si aspetta integrabilità.

---

## Monetizzazione — modello "pay per use"

### `#11` Pannello admin + feature flag premium con sottoscrizioni/licenze
Pannello amministrativo web accessibile solo all'owner (email configurabile via `NUTREEK_OWNER_EMAIL`, oggi hardcoded). Introduce un sistema di **feature flag + entitlement** che permette di marcare le funzionalità ambiziose (#5 micronutrienti, #6 TDEE, #7 auto-plan, #8 coach, #10 API) come premium e veicolarle tramite:
- **Self-hosted**: licenza firmata Ed25519 verificata offline (zero phone-home), sblocca un piano per l'intera istanza
- **Multi-tenant/SaaS**: sottoscrizioni per-utente con scadenza, gestite dal pannello

Piani Free / Pro / Coach. Helper centrale `HasFeature(user, key)`, middleware `requireFeature(...)` per le route, `{{if hasFeature "..."}}` nei template. L'owner ha sempre accesso a tutto.

> Issue GitHub: [#66](https://github.com/easly1989/nutreek/issues/66). Si integra con il RBAC di `#8`.

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

**Raccomandazione ordine di esecuzione**: `#1` → `#2` → `#3` → `#4` → `#6` → `#5` → `#9` → `#7` → `#8` → `#11` → `#10`

> Nota: `#11` (monetizzazione) va dopo che esistono feature premium da vendere e dopo il RBAC di `#8`.

---

## Note tecniche generali

- Stack bloccato: Go 1.23+, chi, SQLite (modernc), HTMX, Tailwind v3.4.17
- Nessuna dipendenza cloud obbligatoria per nessuna feature (inclusa la verifica licenze)
- Ogni feature su branch dedicato `feat/<scope>-<descrizione>` con conventional commits in italiano
- `go build ./...` + `go test ./...` devono essere verdi prima di ogni push

## Issue GitHub (tutte le feature)

| # | Titolo | Sprint | Tipo |
|---|---|---|---|
| [#56](https://github.com/easly1989/nutreek/issues/56) | Filtro allergeni nell'UI | Sprint 1 | quick-win |
| [#57](https://github.com/easly1989/nutreek/issues/57) | Import ricetta da URL (schema.org/Recipe) | Sprint 1 | quick-win |
| [#58](https://github.com/easly1989/nutreek/issues/58) | Tracciamento idratazione | Sprint 1 | quick-win |
| [#59](https://github.com/easly1989/nutreek/issues/59) | PWA installabile + scanner barcode | Sprint 2 | differenziante |
| [#60](https://github.com/easly1989/nutreek/issues/60) | Micronutrienti chiave | Sprint 2 | differenziante |
| [#61](https://github.com/easly1989/nutreek/issues/61) | ⭐ TDEE adattivo (flagship) | Sprint 2 | differenziante |
| [#62](https://github.com/easly1989/nutreek/issues/62) | Generazione automatica del piano | Sprint 3 | ambizioso |
| [#63](https://github.com/easly1989/nutreek/issues/63) | Coach mode multi-cliente (RBAC) | Sprint 3 | ambizioso |
| [#64](https://github.com/easly1989/nutreek/issues/64) | Analytics aderenza + streak | Sprint 3 | ambizioso |
| [#65](https://github.com/easly1989/nutreek/issues/65) | REST API + integrazioni self-hosted | Sprint 3 | ambizioso |
| [#66](https://github.com/easly1989/nutreek/issues/66) | Pannello admin + feature flag + licenze | Monetizzazione | strategico |

