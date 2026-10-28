# Changelog

All notable changes to this project are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

The entries up to 2.0.0 link to issues and pull requests written in Italian: the project switched to English in #146.

---

## [Unreleased]

### Added
- A photo for every recipe (#207): add, change or remove it at the top of the recipe page; the list shows a thumbnail. The page shrinks the picture before sending it, so phone photos come out upright and WebP works; the server keeps a JPEG of at most 1200 px, without EXIF data such as the position, inside the database, so backups include it. Whoever can see the recipe sees its photo; duplicating a recipe copies it (migration 0033)
- Quantities can be changed in place (#206): tap the quantity of a recipe ingredient, or of a food or recipe in the weekly plan, to edit it with Save and Cancel (Enter and Esc work too); the kcal and the totals follow. On a phone the editor wraps under the name; without JavaScript the recipe page opens it as a page
- Recipe ingredients added in pieces stay in pieces: «2 pz» shows as such, with the grams in a tooltip, and counts 2 × the weight per piece in every total, the weekly plan and the shopping list included. Existing rows stay in grams (migration 0035). If an ingredient loses its weight per piece, its rows in pieces turn into grams with the old weight
- Switching between g and pz converts the number, in the picker and in the editors: 1 pz of a 240 g item becomes 240 g, 240 g becomes 1 pz

### Changed
- Quantities above 100,000 are refused when adding or editing a recipe ingredient or a weekly plan item (#206)
- The ingredient forms refuse a weight of one piece that is not a number, instead of dropping it silently; an empty field still means no weight (#206)
- The OpenFoodFacts sync no longer downloads the 13 GB dump every week: it bootstraps from the 0.8 GB CSV export, then applies only the daily changes OpenFoodFacts publishes. It downloads the CSV again when the instance missed more than ~13 days of changes, when the parser changes, or every 30 days, to drop the products OpenFoodFacts deleted. Searches keep answering with the previous products while a download runs, and a failed or suspiciously small one leaves them in place (migration 0034) (#211)
- `NUTREEK_OFF_DUMP_URL` is replaced by `NUTREEK_OFF_CSV_URL` and `NUTREEK_OFF_DELTA_URL`; `NUTREEK_OFF_SYNC_INTERVAL` defaults to `24h`. `POST /admin/refresh-off-dump` forces a CSV download, and the OpenFoodFacts card in Administration shows the last full download, the last daily changes and the date the data reaches (#211)

### Fixed
- The week import keeps the pieces of a CSV exported in pieces, instead of reading them as grams (#206)
- OpenFoodFacts products show their photo and macros again: the Syncer reads the dump's new shape (nutrients in `nutrition.aggregated_set`, the front image from `images.selected.front`, Italian first, then English) as well as the old one, and no longer guesses a French photo when there is none (#204)
- Searching OpenFoodFacts offers only products that can be imported, with all four macros, so picking one no longer fails with «OFF product without complete macros»; a scanned code without macros offers to create the ingredient instead. Exact duplicates (same name, brand and macros) appear once, and every row shows its kcal per 100 g and package size (#204)
- Upgrading: the first start downloads and re-imports the OpenFoodFacts data once (about 0.8 GB); OpenFoodFacts search results are limited until it finishes (#204, #211)
- Clearing the ingredient picker's search brings back what the picker showed before any search, and a slow answer for an older query no longer repaints the results (#205)

---

## [2.1.1] — 2026-10-27

The onboarding wizard keeps your profile, and offers «Recalculate» like the Profile.

### Fixed
- The onboarding wizard keeps the physical data from step 1: step 2 used to clear it when it saved the goals, and accepting the adaptive TDEE in Statistics cleared everything but the calories (#201)
- The wizard's presets dose protein like the Profile, on the adjusted body weight with a BMI of 30 or more (#201)

### Added
- Step 2 of the wizard offers «Recalculate» when the calories change, as the Profile does (#201)

---

## [2.1.0] — 2026-10-19

The interface in English as well as Italian, a public demo, and an instance that tells its owner when a newer release is out.

### Fixed
- A client whose coach assigned them a plan gets that plan's foods suggested in the picker, and can generate a week from it without a plan of their own; before, both looked at the client's own plan only (#186)
- Security: an ingredient's data (name, brand, unit, macros, micronutrients, portion, allergens) can be edited only by whoever created it, or by the owner; everyone else sees it read-only, with "by Bob" in the list. An imported OpenFoodFacts product belongs to whoever imported it; ingredients with no author are edited by the owner, or by anyone when the instance has no owner (#143)
- An ingredient's category is per user: putting a shared ingredient in one of your categories no longer changes it for anyone else, and another user's category is rejected. Existing assignments stay with the category's owner (migration 0028) (#143)

- English: the last Italian leftovers found by the page-by-page check (#160, part of #141): the default name of a nutrition plan, and the reasons the week's messages quote when an OpenFoodFacts import or an addition fails

### Added
- Translation infrastructure (#151, part of #141): the interface text moves into per-language catalogs (`internal/i18n/locales`), Italian and English, starting with the shared layout and the Ingredients section. Italian stays the only language users see; English can be tried with the `lang=en` cookie until every section is translated

- Choosing the language (#159, part of #141): Italian, English or "like the browser" in Settings → Appearance and language, saved in the account; a selector on the signed-out pages, kept in a cookie. Without a choice the browser's languages decide, with English as the fallback. Existing users stay in Italian (migration 0030)

- A guide for community translators (#162, part of #141): `CONTRIBUTING.md`, `docs/translating.md` with a glossary, and a "New language" issue template. A language other than Italian and English is marked "beta" in the language selectors; a key it lacks shows the English text, and CI only warns about it, listing the keys (`go run ./cmd/i18ncheck`)

- The README and the public home in both languages (#161, part of #141): the English README says how to pick a language and shows English screenshots (`docs/screenshots/en/`), the Italian one keeps the Italian ones; the home's product image follows the page's language

- The demo build (#182, part of #137): `go build -tags demo` makes a public demo where the home is an entry page with one button. It creates a throwaway account with a complete profile, in the visitor's language, with a band on every page and a notice at first. Sign-in, sign-up, password change and account deletion are not mounted; sessions end after 2 hours without use (`NUTREEK_DEMO_IDLE`). The demo refuses to start on a database with real users. The normal binary does not contain any of it

- The demo cleans up after itself (#184, part of #137): a demo group nobody has used for `NUTREEK_DEMO_IDLE` (2 hours) is deleted with its users and all their data, and every group is deleted each night at 03:00 UTC. An ingredient a deleted user made that another group still uses stays, with no author. Sessions of a deleted demo stop working. The demo keeps no volume: a new deploy starts from an empty database

- The demo's lockdown (#185, part of #137): the admin pages, API tokens and the REST API, recipe import from a URL and the owner's OpenFoodFacts sync are not mounted, and automatic backups are off. Limits, each with a variable: 200 live demo users, then the entry says the demo is full (`NUTREEK_DEMO_MAX_USERS`); 10 demos started per address per hour (`NUTREEK_DEMO_STARTS_PER_HOUR`); 1 MB request bodies (`NUTREEK_DEMO_MAX_BODY`); 300 hand-made ingredients and 100 recipes per user (`NUTREEK_DEMO_MAX_INGREDIENTS`, `NUTREEK_DEMO_MAX_RECIPES`). Hand-made ingredients and public recipes are seen and used only inside the visitor's demo group; OpenFoodFacts products are shared, and read-only. Normal installs keep the shared catalog

- The demo account starts lived-in (#183, part of #137): foods, a few OpenFoodFacts demo products with a barcode, recipes, a nutrition plan with variants for each meal, the current week and the 12 before it with cheats, water, workouts and weigh-ins, and this week's shopping list, all dated from today and named in the visitor's language. The same seed dresses a local database for the README screenshots: `go run -tags demo ./cmd/nutreek -demo-seed you@example.com -demo-seed-lang en` fills an account that has no meals yet

- Three ways into the demo (#186, part of #137): "Follow a plan", "You're a coach" and "You're a coach's client". The coach and client entries create a whole group in the visitor's language: a coach with a library of three plans, two clients on plans from it with different adherence, and one more person to add. The visitor gets 12 weeks of history, the others 4, and the group shares one set of hand-made foods. The coach lands on the coach panel, the client on their week, on the plan the coach assigned. In the demo a coach adds clients by choosing among their group, not by email

- The demo's image and deploy (#188, part of #137): `docker build --build-arg DEMO=1` builds the demo; the Docker demo workflow publishes `ghcr.io/easly1989/nutreek:demo` and `:demo-<version>` (amd64) with every release and by hand, then deploys it on Northflank. CI builds the demo binary and runs an end-to-end test on it. The image's `/data` now belongs to the runtime user, so it runs without a volume. Setup in `docs/demo.md`

- "View as" in the demo (#187, part of #137): in a coach or client demo, a menu in the band signs the visitor in as anyone of their group (themselves, the coach, a client, the person still without a coach) with no password, to see both sides. It reaches only the visitor's own group; anyone else is not found

- The live demo is linked (#189, part of #137): a "Try the live demo" button and a line in both READMEs, at https://p01--nutreek--7dcjw6mlmtky.code.run/. `docs/demo.md` says why v2.0.0, tagged before the demo workflow existed, did not publish the demo image

- Update notices (#167): every stable release is recorded in `versions.json` on the `site-data` branch. A release build reads it a minute after start and then once a day, and tells the owner of a newer version with a card in the sidebar (at the top of «More» on the phone) and a «How to update» dialog, also at `/admin/update`. *Administration → Updates* shows the installed version and the last check, with «Check now» and a switch for the daily check, kept in the new `instance_settings` table (migration 0031); `NUTREEK_UPDATE_CHECK=false` turns it off for the install. A page left open across an update offers **Reload** / **Later** on its next HTMX request. The README has an «Updating» section. The demo never checks

### Changed
- The REST API v1 error messages are in English, whatever the user's language (#159)
- The code, docs, commits, issues and pull requests are in English, this CHANGELOG included; the app itself is still in Italian (#146)
- The README screenshots and the public home's product image come from the demo seed (#189, part of #137): English food and meal names in the English ones, Italian in the Italian ones. On the way, the demo gives every account an active weekly template, so its nutrition plan is no longer empty, and shows the OpenFoodFacts demo products of the visitor's language only (a barcode typed in still finds the others)

---

## [2.0.0] — 2026-10-16

First release published as an image on GHCR: the «Sereno» restyle, an installable app with a barcode scanner, and the fixes from the catch-up round.

### Added
- Docker image on GHCR published with every release (`:latest`, `:vX.Y.Z`, `:vX.Y`), `nutreek -version` flag (#111)
- Barcode scanner in the meal, recipe and nutrition plan pickers (#59): the camera reads the code (native BarcodeDetector, otherwise a ZXing reader served by the app), and the product opens ready to add, looked up first among your ingredients and in the local OpenFoodFacts copy; when it is missing, "Create the ingredient" creates it with the code attached. The digits can also be typed, and search recognizes a typed EAN-8/13 or UPC-A code.
- Nutreek installs on the phone as an app (#59): manifest and icons in the theme color, a service worker caching only static files, an "App on your phone" section in Settings (an "Install the app" button, steps for Safari on iPhone). The camera and installing need https.

### Fixed
- In the picker an OpenFoodFacts product no longer opens on "1 pc", which meant the whole package (500 g of oat flakes in one click). The proposed quantity is the OFF serving in g/ml (`serving_quantity`, stored in the local copy and in imported ingredients); when the serving is unknown the field stays empty, in grams. Ingredients created by hand do not change (#136)
- OpenFoodFacts product photos show again in the Ingredients list and in the pickers: the CSP was blocking them. Only `images.openfoodfacts.org` is allowed, photos load without a referrer, and when a photo is missing the barcode icon stays instead of an empty square (#133)
- A code OpenFoodFacts does not know (the API answers 404) is "not found", no longer a service error (#59)
- Security: the login rate limit can no longer be bypassed with a made-up `X-Forwarded-For`. The header counts only when the connection comes from a proxy on loopback or a private network; an invalid `X-Request-ID` is replaced (#115)
- Two concurrent transactions that read and then write no longer fail with `database is locked`: SQLite transactions take the write lock immediately (`_txlock=immediate`) and queue up (#116)
- Weigh-ins no longer lose their date on read (they all came back as 1/1/0001); the Statistics table no longer uses a nonexistent field of the energy requirement (#123)
- Security: variants and nutrition plan items can only be created in your own plan, and items (plan, meals, cheats) can only use your own or public recipes; other users' ids answer 404 (#129)
- Items in pieces ("pc") count as quantity × the ingredient's unit weight in the week, in automatic generation and in the shopping list; before, they counted as grams (#126)
- Saving an edited ingredient no longer clears its category; errors in the quick "New ingredient / New recipe" forms now show in the dialog instead of being dropped; an error while adding a food reloads the week instead of nesting it inside the day (#122)

### Changed
- Nutreek is open source: **GNU AGPL v3.0** license instead of the proprietary one. The README is in English, the Italian one is in `README.it.md`; the image on GHCR is public.
- «Sereno» restyle, part one (#121): a new theme system (light / dark / system mode × forest / sea / sunset / graphite color, stored in a cookie and read by the server), the Figtree font served by the app, a sidebar on desktop and a bottom bar on the phone with the "More" sheet, an "Appearance" menu. The week shows one day at a time: day tabs, a summary with a kcal ring and macros, meals, water and workouts; the open day is in the URL (`?day=`). Old `theme` cookies are converted.
- «Sereno» restyle, part two (#122): recipes with live search, filters, kcal and macros per serving and the energy split in the editor; ingredients with filters by source, category and allergen and a form in sections; the nutrition plan in tabs (meals and variants / weekly templates, replacing `/templates`) with kcal per variant; the template editor by day; the "Add food" picker opens in a dialog over the week (a bottom sheet on the phone) with a kcal preview and g / pc units.
- «Sereno» restyle, part three (#123): the shopping list by aisle with a cart, progress, whole pieces and two-column printing; Statistics with summary cards and SVG charts in the theme colors (Chart.js removed); Profile with the macro split and presets, Weight with 30/90/365-day ranges, Settings with a side index and a theme preview; Administration in tabs (features inside the Overview, `/admin/panel/features` redirects there), a Coach panel and a Premium page that says which feature was locked (`/upgrade?da=`); sign-in and sign-up with Show/Hide password, a full-page first-run setup with presets computed from the energy requirement, a public home with a preview of the week, new 404/500 pages. The OpenFoodFacts sync is managed from Administration, no longer from Profile.

### Maintenance
- Go 1.27.2; chi 5.5, goose 3.29, modernc sqlite 1.60, x/crypto 0.58; SortableJS 1.15.7; GitHub Actions on current majors, golangci-lint pinned to v2.14.0; base images `golang:1.27-alpine` and `alpine:3.24`; `.gitattributes` with LF line endings (#112)
- No scripts from external hosts: the CSP no longer allows `cdn.jsdelivr.net` (Chart.js, first vendored and then replaced by the SVG charts of #123). Removed `sqlc.yaml`, which generated nothing: the SQL is hand-written in the repositories (#117)
- README with badges and a "Support the project" section, `.github/FUNDING.yml` for the "Sponsor" button, new screenshots in the Sereno style (#124)

---

## [Round 5] — 2026-06-12

### Added
- **Coach mode** (#63): a nutritionist plan library, assignment to clients, frozen plan history, a coach dashboard with adherence per client
- **REST API v1** (#65): Bearer token authentication, endpoints `GET /me`, `GET /week`, `GET /ingredients`, `GET /activities`, `POST /water`, `POST /weight`, `POST /workout`; token management in `/settings`
- **Admin panel** (#66): an owner-only `/admin/panel` managing feature flags (enable/disable/premium), the user list, premium subscriptions
- **Feature flag system**: a `features` table with kill switch + is_premium, `requireFeature(key)` middleware, `HasFeature(ctx, user, key)` helper, 60 s TTL cache
- **Subscriptions**: a `subscriptions` table with start/end dates, an `IsActive` method, managed from the admin panel
- **API tokens**: an `api_tokens` table (SHA-256), created/revoked from settings, `requireAPIToken` middleware, `LastUsedAt` updated on every use

### Technical
- Migrations 0023 (coach), 0024 (subscriptions/features), 0025 (api_tokens)
- New files in `internal/db`: `coach.go`, `api_tokens.go`, `features.go`, `subscriptions.go`
- CSRF exempt on `/api/*` and `/admin/*`
- Structured JSON responses for every API endpoint

---

## [Round 4] — 2026

### Added
- **Automatic plan generation** (#62): a greedy algorithm that fills the week from the nutritionist variants, aiming at the macro targets
- **Adherence analytics** (#64): daily/weekly adherence score, streaks on the plan, compliance % per meal type, charts on the statistics page

### Technical
- New packages: `internal/planner/`, `internal/service/planner.go`
- A "Generate week" form on the dashboard (shown only for an empty week with an active plan)

---

## [Round 3] — 2026

### Added
- **Adaptive TDEE** (#61): estimates the real TDEE from the weight/intake trend of the last weeks, suggests adjusting the macro targets, an "Accept" button on the statistics page
- **Micronutrients** (#60): fiber, sugars, saturated fat and sodium extracted from the OFF dump; shown in the ingredient and recipe detail

### Technical
- New packages: `internal/tdee/`, `internal/service/tdee.go`
- New fields on `ingredients`: `fiber_g`, `sugar_g`, `saturated_fat_g`, `sodium_mg`

---

## [Round 2] — 2026

### Added
- **Hydration tracking** (#58): a daily `water_logs` log, a widget in the day card, a ml target in the profile, a sparkline in the statistics
- **Recipe import from a URL** (#57): parses `schema.org/Recipe` (JSON-LD and microdata), pre-fills the recipe form
- **Allergen filter** (#56): allergen tags on the ingredient card, a multi-allergen filter in the ingredient list, inline warnings in the plan and the pickers

### Technical
- New package: `internal/recipeimport/`
- `water_logs` table migration
- 14 predefined allergens (EU list)

---

## [Round 1] — 2026

### Added
- **Local OFF search over FTS5**: weekly sync of the OFF dump (~250k Italian products), searches under 50 ms, no network dependency at runtime
- **User profile**: weight/height, computed BMI/BMR, personal macro targets
- **Workouts**: workout log with calories burned, a weekly activity dashboard
- **UI themes**: forest/ocean/sunset/mono, kept in a cookie
- **Pinned sidebar**: a collapsible side navigation, pinned state kept
- **Guided tour**: an onboarding dialog for new users
- **Statistics charts**: weight, hydration and weekly macro charts with chart.js
- **Nutrition plan**: a hierarchy of meal types → variants → items, assigned to the user

### Technical
- `internal/off/dump.go`: a Syncer that stream-parses the JSONL.gz dump
- Migrations 0001–0022
- Per-IP rate limiting on login
- Security headers (HSTS, CSP, X-Frame-Options)
- CSV export/import of weekly plans
- User data export as JSON (GDPR Art. 20)
- Account deletion (GDPR Art. 17)
