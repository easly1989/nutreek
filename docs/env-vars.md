# Environment variables

Moved from CLAUDE.md. Read before adding or changing a NUTREEK_* setting.

| Variable | Default | Notes |
|-----------|---------|------|
| `NUTREEK_ADDR` | `:8080` | |
| `NUTREEK_DB_PATH` | `./data/nutreek.db` | |
| `NUTREEK_ENV` | `dev` | `prod` enables secure cookies |
| `NUTREEK_SESSION_KEY` | auto in dev | **required in prod** (hex ≥64 chars) |
| `NUTREEK_OFF_BASE_URL` | `https://world.openfoodfacts.org` | live product lookup host (barcode) |
| `NUTREEK_OFF_CSV_URL` | `https://static.openfoodfacts.org/data/en.openfoodfacts.org.products.csv.gz` | CSV export the Syncer bootstraps from |
| `NUTREEK_OFF_DELTA_URL` | `https://static.openfoodfacts.org/data/delta/` | folder of `index.txt` and of the daily deltas |
| `NUTREEK_OFF_SYNC_INTERVAL` | `24h` | minimum interval of the automatic sync |
| `NUTREEK_OFF_SYNC_ENABLED` | `true` | enables the sync scheduler |
| `NUTREEK_RECONCILE_INTERVAL` | `24h` | |
| `NUTREEK_RECONCILE_ENABLED` | `true` | |
| `NUTREEK_ADMIN_TOKEN` | (none) | token for the legacy admin endpoints |
| `NUTREEK_OWNER_EMAIL` | (none) | owner's email — sole access to `/admin/panel` |
| `NUTREEK_PREMIUM_ENABLED` | `false` | when `true`, gates the features marked premium |
| `NUTREEK_BACKUP_ENABLED` | `false` | enables periodic automatic SQLite backups |
| `NUTREEK_BACKUP_DIR` | `./data/backups` | where backups are written |
| `NUTREEK_BACKUP_INTERVAL` | `24h` | automatic backup interval |
| `NUTREEK_BACKUP_KEEP` | `7` | backups to keep (the oldest are deleted) |
| `NUTREEK_UPDATE_CHECK` | `true` | release builds read `versions.json` (branch `site-data`) once a day and tell the owner of a newer release; the owner's switch lives in `instance_settings` |
| `NUTREEK_SECRET_KEY` | (none) | at least 16 characters (`openssl rand -hex 32`); seals the tokens of the store plugins (#239) and the secrets of the authenticator app (#335) in the database. Without it no store can be connected and the authenticator app cannot be turned on. Changing it makes the stored tokens and secrets unreadable: connect the stores again, and turn the authenticator app off and on again (the recovery codes still work) |
| `NUTREEK_STORES_ALLOW_PRIVATE` | `false` | lets a store plugin sit on a private address and speak plain http (a home server); by default only public https addresses are reached, as a plugin address is any URL a user gives. **It opens the instance's private network to every account**, not only the admin |
| `NUTREEK_BASE_URL` | (none) | the public address of the instance, e.g. `https://nutreek.example.com` (no path). The sign-in callback and the links in emails are built on it, and passkeys (#336) take their site name from it: without it no passkey can be added or used. Needed for `NUTREEK_OIDC_ISSUER` |
| `NUTREEK_OIDC_ISSUER` | (none) | turns on "Sign in with <name>" (#333) through an OpenID Connect provider (Authelia, Keycloak, Authentik). Needs `NUTREEK_BASE_URL`, `NUTREEK_OIDC_CLIENT_ID` and `NUTREEK_OIDC_CLIENT_SECRET`. Register the client with the redirect address `<NUTREEK_BASE_URL>/auth/oidc/callback`, scopes `openid profile email`, authorization code with PKCE (S256). The provider must send a verified `email` (in the ID token or in userinfo) |
| `NUTREEK_OIDC_CLIENT_ID` | (none) | the client id registered at the provider |
| `NUTREEK_OIDC_CLIENT_SECRET` | (none) | the client secret |
| `NUTREEK_OIDC_NAME` | `Single sign-on` | the label on the button |
| `NUTREEK_OIDC_AUTO_CREATE` | `false` | `true` creates an account for an email nobody has used; otherwise only existing accounts, matched by email, can sign in |
| `NUTREEK_SMTP_HOST` | (none) | turns on email (#334): the login page gets "Forgot password?" and a reset link is mailed. Needs `NUTREEK_BASE_URL` (the links are built on it) and `NUTREEK_SMTP_FROM`. Nothing is sent without it |
| `NUTREEK_SMTP_PORT` | `587` | the server's port: 587 (STARTTLS) or 465 (TLS from the start) |
| `NUTREEK_SMTP_USER` | (none) | sign-in name at the server; empty = no sign-in |
| `NUTREEK_SMTP_PASSWORD` | (none) | the password for it. Plain sign-in is only sent over an encrypted connection (or to localhost) |
| `NUTREEK_SMTP_FROM` | (none) | the sender, `Nutreek <nutreek@example.com>` or a bare address |
| `NUTREEK_SMTP_TLS` | `starttls` | `starttls` (refuses a server that does not offer it), `tls` (port 465) or `none` (a relay on your own machine or network) |
| `NUTREEK_TRUSTED_PROXIES` | (not set) | the reverse proxies whose `X-Forwarded-For` is believed when limiting tries (#346): addresses and ranges, comma-separated, e.g. `172.18.0.2,10.0.0.0/24`; `none` believes nobody. **Not set, only the loopback address is believed** (a proxy on the same host); a proxy in another container or machine must be listed, or every visitor counts as the proxy and five sign-ins a minute is the limit for everyone (the log warns about it once). List only proxies: a range that also holds ordinary hosts lets them pick their address (`0.0.0.0/0` is refused). Give a proxy container a fixed address. When Docker publishes the app's port itself (no DNAT, so the connection is forwarded by Docker), the peer the app sees is the Docker network's gateway, not the reverse proxy: list the address the warning in the log names (for example `172.24.0.1`). Anyone who can reach that published port directly can then pick their address |
| `NUTREEK_DEMO_IDLE` | `2h` | demo build only: a demo session ends, and its group is deleted, after this long without use; every group is deleted nightly at 03:00 UTC |
| `NUTREEK_DEMO_MAX_USERS` | `200` | demo build only: live demo users; then the entry says the demo is full |
| `NUTREEK_DEMO_STARTS_PER_HOUR` | `10` | demo build only: demos started per client address per hour; then 429 |
| `NUTREEK_DEMO_MAX_BODY` | `1048576` | demo build only: request body in bytes; then 413 |
| `NUTREEK_DEMO_MAX_INGREDIENTS` | `300` | demo build only: hand-made ingredients per user |
| `NUTREEK_DEMO_MAX_RECIPES` | `100` | demo build only: recipes per user |
