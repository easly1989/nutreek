# Nutreek — Roadmap

> Historical: this page covers the rounds that led to the 2.0.0 release. Current work is tracked in [GitHub issues](https://github.com/easly1989/nutreek/issues), and the issues and pull requests linked below are written in Italian.

> Positioning: **"The self-hosted nutrition operating system"** — the only privacy-first tool covering both adaptive personal tracking and the nutritionist↔client workflow, with no cloud and no subscriptions.

---

## Progress

| Sprint | Status | Notes |
|--------|-------|------|
| Sprint 1 — Quick wins | ✅ Done | Rounds 1–3 |
| Sprint 2 — Differentiators | ✅ Done | Rounds 3–4 |
| Sprint 3 — Ambitious | ✅ Done | Rounds 4–5 |
| Monetization | ✅ Done | Round 5 |
| Round 6 — Docs & gating | ✅ Done | #68, #69, #70 |

---

## Existing differentiators

| Asset | Advantage over competitors |
|---|---|
| Local OFF search over FTS5 (<50 ms, no rate limit) | Faster and more reliable than MyFitnessPal/Cronometer |
| Cheat tracking with effective macro overrides | Nearly unique: tells "planned" apart from "actually eaten" |
| Hierarchical nutrition plans (meal types → variants → items) | A niche no self-hosted app serves |
| Weekly templates + multi-week shopping list | On par with Mealime/Paprika |
| Adaptive TDEE from the weight/intake trend | MacroFactor sells this feature at $12/month |
| Coach mode: plan library, client assignment, frozen history | Not served by any self-hosted app |
| REST API v1 with Bearer tokens | Integration with Home Assistant, n8n, personal scripts |
| Self-hosted, ~20 MB binary, zero cloud, zero CGO | A structural moat against the whole cloud market |

---

## Sprint 1 — Quick wins ✅

### `#1` Allergen filter in the UI — ✅ [#56](https://github.com/easly1989/nutreek/issues/56)
Allergen tags on the ingredient card, a filter in the list, inline warnings in the plan and the pickers.

### `#2` Recipe import from a URL — ✅ [#57](https://github.com/easly1989/nutreek/issues/57)
Paste a link to any recipe site: parses the `schema.org/Recipe` data (JSON-LD or microdata) and pre-fills name, ingredients and servings.

### `#3` Hydration — ✅ [#58](https://github.com/easly1989/nutreek/issues/58)
`water_logs` table, a widget in the day card, a daily target in the profile, a sparkline in the stats.

---

## Sprint 2 — Differentiators ✅

### `#4` PWA + camera barcode scanner — ✅ [#59](https://github.com/easly1989/nutreek/issues/59)
Service worker, web manifest and BarcodeDetector. Deferred at first, shipped in 2.0.0.

### `#5` Key micronutrients — ✅ [#60](https://github.com/easly1989/nutreek/issues/60)
Fiber, sugars, saturated fat and sodium from the OFF dump. Extended `ingredients`, `macros.go` and the UI.

### `#6` ⭐ Adaptive TDEE (MacroFactor style) — ✅ [#61](https://github.com/easly1989/nutreek/issues/61)
Estimates the real TDEE from the weekly weight-vs-intake trend and suggests adjusting the macro targets.

---

## Sprint 3 — Ambitious ✅

### `#7` Automatic plan generation — ✅ [#62](https://github.com/easly1989/nutreek/issues/62)
A greedy algorithm: given the targets and the pool of nutrition plan variants, it fills the week hitting the macros.

### `#8` Multi-client coach mode — ✅ [#63](https://github.com/easly1989/nutreek/issues/63)
Plan library, client assignment, frozen history, a coach dashboard with adherence. RBAC implemented.

### `#9` Adherence analytics (plan vs actual) — ✅ [#64](https://github.com/easly1989/nutreek/issues/64)
Daily/weekly adherence score, streaks on the plan, compliance % per meal type.

### `#10` REST API + automations — ✅ [#65](https://github.com/easly1989/nutreek/issues/65)
Bearer token API v1: `GET /me`, `GET /week`, `GET /ingredients`, `GET /activities`, `POST /water`, `POST /weight`, `POST /workout`.

---

## Monetization ✅

### `#11` Admin panel + premium feature flags with subscriptions — ✅ [#66](https://github.com/easly1989/nutreek/issues/66)
An owner-only web panel (`/admin/panel`). Feature flags with kill switch + is_premium. Per-user subscriptions. `requireFeature` middleware. `HasFeature` helper. 60 s cache with invalidation.

Premium features: `auto_plan`, `tdee_adaptive`, `coach_mode`, `api_access`.

---

## Round 6 — Docs & actual gating ✅

### `#68` Updated documentation — ✅
CLAUDE.md (new env vars, new packages, premium section), ROADMAP.md (this file), CHANGELOG.md.

### `#69` README with screenshots — ✅
README updated with the full feature table, env vars, REST API examples, the Admin & Premium section.

### `#70` Actual premium gating — ✅
- `requireFeature("api_access")` added to the `/api/v1/*` group
- `POST /settings/api-tokens` gated with `requireFeature("api_access")`
- `Features map[string]bool` in `authPageData`, filled in `baseData()` on every request
- "Premium" badges in the templates (dashboard, stats, settings)
- JSON 403 response from the `requireFeature` middleware for API routes

---

## Impact / effort matrix

```
High impact
     │
  #6 │         #8   #11
  #4 │    #7       #10
  #2 │  #5   #9
  #1 │        #3
     └─────────────────── High effort
      low           high
```

Order followed: `#1` → `#2` → `#3` → `#5` → `#6` → `#9` → `#7` → `#8` → `#11` → `#10` → `#68/69/70`

---

## General technical notes

- Stack: Go, chi, SQLite (modernc), HTMX, Tailwind v3.4.17
- No feature requires a cloud dependency
- Each feature on its own branch `feat/<scope>-<description>` with Conventional Commits (in Italian at the time; in English since #146)
- `go build ./...` + `go test ./...` must be green before every push

---

## GitHub issues (every feature)

| # | Title | Sprint | Status |
|---|---|---|---|
| [#56](https://github.com/easly1989/nutreek/issues/56) | Allergen filter in the UI | Sprint 1 | ✅ Closed |
| [#57](https://github.com/easly1989/nutreek/issues/57) | Recipe import from a URL (schema.org/Recipe) | Sprint 1 | ✅ Closed |
| [#58](https://github.com/easly1989/nutreek/issues/58) | Hydration tracking | Sprint 1 | ✅ Closed |
| [#59](https://github.com/easly1989/nutreek/issues/59) | Installable PWA + barcode scanner | Sprint 2 | ✅ Closed (2.0.0) |
| [#60](https://github.com/easly1989/nutreek/issues/60) | Key micronutrients | Sprint 2 | ✅ Closed |
| [#61](https://github.com/easly1989/nutreek/issues/61) | ⭐ Adaptive TDEE (flagship) | Sprint 2 | ✅ Closed |
| [#62](https://github.com/easly1989/nutreek/issues/62) | Automatic plan generation | Sprint 3 | ✅ Closed |
| [#63](https://github.com/easly1989/nutreek/issues/63) | Multi-client coach mode (RBAC) | Sprint 3 | ✅ Closed |
| [#64](https://github.com/easly1989/nutreek/issues/64) | Adherence analytics + streaks | Sprint 3 | ✅ Closed |
| [#65](https://github.com/easly1989/nutreek/issues/65) | REST API + self-hosted integrations | Sprint 3 | ✅ Closed |
| [#66](https://github.com/easly1989/nutreek/issues/66) | Admin panel + feature flags + subscriptions | Monetization | ✅ Closed |
| [#67](https://github.com/easly1989/nutreek/issues/67) | Round 5 roadmap tracking | Tracking | ✅ Closed |
| [#68](https://github.com/easly1989/nutreek/pull/68) | Updated docs (CLAUDE.md, ROADMAP, CHANGELOG) | Round 6 | ✅ Merged |
| [#69](https://github.com/easly1989/nutreek/pull/69) | README with screenshots | Round 6 | ✅ Merged |
| [#70](https://github.com/easly1989/nutreek/pull/70) | Actual premium gating | Round 6 | ✅ Merged |
