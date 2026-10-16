# Changelog

Tutte le modifiche notevoli a questo progetto sono documentate in questo file.

Il formato segue [Keep a Changelog](https://keepachangelog.com/it/1.1.0/).

---

## [Non rilasciato]

### Corretto
- Sicurezza: i dati di un ingrediente (nome, marca, unità, macro, micronutrienti, porzione, allergeni) li modifica solo chi l'ha creato, o l'owner; gli altri lo vedono in sola lettura, con «di Bob» in lista. Un prodotto OpenFoodFacts importato ha come autore chi l'ha importato; quelli senza autore li modifica l'owner, o chiunque se l'istanza non ne ha uno (#143)
- La categoria di un ingrediente è di ogni utente: mettere un ingrediente condiviso in una propria categoria non cambia più quella degli altri, e una categoria di un altro utente viene rifiutata. Le assegnazioni esistenti restano a chi possiede la categoria (migrazione 0028) (#143)

---

## [2.0.0] — 2026-10-16

Prima release pubblicata come immagine su GHCR: restyle «Sereno», app installabile con scanner del codice a barre, e le correzioni del giro di ripresa.

### Aggiunto
- Immagine Docker privata su GHCR pubblicata a ogni release (`:latest`, `:vX.Y.Z`, `:vX.Y`), flag `nutreek -version` (#111)
- Scanner del codice a barre nei picker di pasto, ricetta e piano nutrizionale (#59): la fotocamera legge il codice (BarcodeDetector nativo, altrimenti un lettore ZXing servito dall'app), il prodotto si apre già pronto da aggiungere, cercato prima tra i propri ingredienti e nella copia locale di OpenFoodFacts; se manca, «Crea l'ingrediente» lo crea con il codice collegato. Le cifre si possono anche scrivere a mano, e la ricerca riconosce un codice EAN-8/13 o UPC-A digitato.
- Nutreek si installa sul telefono come app (#59): manifest e icone nel colore del tema, service worker con la cache dei soli file statici, sezione «App sul telefono» in Impostazioni (bottone «Installa l'app», passaggi per Safari su iPhone). Fotocamera e installazione richiedono https.

### Corretto
- Nel picker un prodotto OpenFoodFacts non si apre più su «1 pz», che valeva l'intera confezione (500 g di fiocchi d'avena con un clic). La quantità proposta è la porzione OFF in g/ml (`serving_quantity`, salvata nella copia locale e negli ingredienti importati); se la porzione non è nota il campo resta vuoto, in grammi. Gli ingredienti creati a mano non cambiano (#136)
- Le foto dei prodotti OpenFoodFacts si vedono di nuovo nella lista Ingredienti e nei picker: la CSP le bloccava. È consentito solo `images.openfoodfacts.org`, le foto si caricano senza referrer, e se una foto manca resta l'icona del codice a barre invece di un quadrato vuoto (#133)
- Un codice sconosciuto a OpenFoodFacts (l'API risponde 404) è «non trovato» e non più un errore del servizio (#59)
- Sicurezza: il rate limit del login non si aggira più con un `X-Forwarded-For` inventato. L'header vale solo se la connessione arriva da un proxy in loopback o in rete privata; un `X-Request-ID` non valido viene sostituito (#115)
- Due transazioni concorrenti che leggono e poi scrivono non falliscono più con `database is locked`: le transazioni SQLite prendono subito il lock di scrittura (`_txlock=immediate`) e si accodano (#116)
- Le pesate non perdono più la data in lettura (risultavano tutte all'1/1/0001); la tabella delle Statistiche non usa più un campo inesistente del fabbisogno (#123)
- Sicurezza: varianti e voci del piano nutrizionale si creano solo nel proprio piano, e nelle voci (piano, pasti, sgarri) si possono usare solo ricette proprie o pubbliche; gli id di altri utenti rispondono 404 (#129)
- Le voci in pezzi («pz») valgono quantità × peso unitario dell'ingrediente nella settimana, nella generazione automatica e nella lista della spesa; prima contavano come grammi (#126)
- Salvare un ingrediente modificato non azzera più la sua categoria; gli errori dei form rapidi «Nuovo ingrediente / Nuova ricetta» ora si vedono nella finestra invece di essere scartati; un errore aggiungendo un alimento ricarica la settimana invece di annidarla nel giorno (#122)

### Cambiato
- Nutreek è open source: licenza **GNU AGPL v3.0** al posto di quella proprietaria. Il README è in inglese, l'italiano è in `README.it.md`; l'immagine su GHCR è pubblica.
- Restyle «Sereno», prima parte (#121): nuovo sistema di temi (modalità chiaro / scuro / come il sistema × colore bosco / mare / tramonto / grafite, in cookie e letto dal server), font Figtree servito dall'app, sidebar su desktop e barra in basso su telefono con il foglio «Altro», menu «Aspetto». La settimana mostra un giorno alla volta: schede dei giorni, riepilogo con anello kcal e macro, pasti, acqua e allenamenti; il giorno aperto è nell'URL (`?day=`). I vecchi cookie `theme` vengono convertiti.
- Restyle «Sereno», seconda parte (#122): ricette con ricerca live, filtri, kcal e macro per porzione e ripartizione energetica nell'editor; ingredienti con filtri per provenienza, categoria e allergene e form a sezioni; piano nutrizionale a schede (pasti e varianti / template settimanali, che sostituisce `/templates`) con kcal per variante; editor dei template per giorno; il picker «Aggiungi alimento» si apre in una finestra sopra la settimana (foglio dal basso sul telefono) con anteprima kcal e unità g / pz.
- Restyle «Sereno», terza parte (#123): lista della spesa per reparto con carrello, avanzamento, pezzi interi e stampa in due colonne; Statistiche con card di sintesi e grafici SVG nei colori del tema (Chart.js rimosso); Profilo con ripartizione dei macro e modelli, Peso con intervallo 30/90/365 giorni, Impostazioni con indice laterale e anteprima dei temi; Amministrazione a schede (funzionalità dentro la Panoramica, `/admin/panel/features` rimanda lì), Pannello coach e pagina Premium che dice quale funzionalità era bloccata (`/upgrade?da=`); accesso e registrazione con Mostra/Nascondi password, configurazione iniziale a tutta pagina con i modelli calcolati dal fabbisogno, home pubblica con anteprima della settimana, pagine 404/500 nuove. La sincronizzazione OpenFoodFacts si gestisce dall'Amministrazione e non più dal Profilo.

### Manutenzione
- Go 1.27.2; chi 5.5, goose 3.29, modernc sqlite 1.60, x/crypto 0.58; SortableJS 1.15.7; GitHub Actions alle major correnti, golangci-lint fissato a v2.14.0; immagini base `golang:1.27-alpine` e `alpine:3.24`; `.gitattributes` con fine riga LF (#112)
- Nessuno script da host esterni: la CSP non ammette più `cdn.jsdelivr.net` (Chart.js, prima vendorizzato e poi sostituito dai grafici SVG di #123). Tolto `sqlc.yaml`, che non generava nulla: il SQL è scritto a mano nei repo (#117)
- README con badge e sezione «Supporta il progetto», `.github/FUNDING.yml` per il bottone «Sponsor», screenshot nuovi nello stile Sereno (#124)

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
