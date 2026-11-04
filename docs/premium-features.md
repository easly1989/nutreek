# Premium features

Moved from CLAUDE.md. Read before touching HasFeature, requireFeature, feature flags or subscriptions.

### `HasFeature` logic

`app.HasFeature(ctx, user, key) bool` — the single entry point for premium access:

- `PremiumEnabled=false` → **always `true`** (everything available, dev default)
- The user is the owner (`isOwner`) → **always `true`**
- `feature.enabled = false` → **`false`** for everyone (kill switch)
- `feature.is_premium = false` → **`true`** (public feature, enabled)
- `feature.is_premium = true` → **`Subscriptions.IsActive(userID)`**
- DB error or unknown feature → `true` (permissive fallback)

### `requireFeature` middleware

`app.requireFeature(key)` — chi middleware that protects routes:

- Browser HTML request → `302 /upgrade`
- HTMX request (`HX-Request: true`) → `HX-Redirect: /upgrade` + `403`
- API/JSON request → `403 {"error": "feature not available on your plan"}`

### Cache

`featureCache` — `sync.RWMutex` + 60 s TTL, invalidated after every admin toggle (`invalidateFeatureCache()`).

### Feature keys in use

| Key | Feature | Protected routes |
|-----|-------------|---------------|
| `auto_plan` | Automatic week generation | `POST /week/generate` |
| `tdee_adaptive` | Adaptive TDEE | `POST /tdee/accept` |
| `coach_mode` | Coach panel | group `/coach/*` |
| `api_access` | REST API v1 | group `/api/v1/*`, `POST /settings/api-tokens` |

### `Features` in templates

`authPageData.Features map[string]bool` — filled in `baseData()` on every authenticated request (cheap: uses the 60 s cache). Templates use `{{if index .Features "key"}}` to show Premium badges and upgrade messages before the user clicks.
