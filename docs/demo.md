# The public demo

The demo (#137) is a different build of the same app: `go build -tags demo ./cmd/nutreek`, or the Docker image with `--build-arg DEMO=1`. Every visitor gets a throwaway, lived-in account with no sign-up; sign-in, admin, the REST API and recipe import are not mounted, and demo groups are deleted after 2 hours without use and every night at 03:00 UTC. The normal binary and image contain none of it. The `NUTREEK_DEMO_*` limits are in the environment variables table of `CLAUDE.md`.

It runs on [Northflank](https://northflank.com), from the public image `ghcr.io/easly1989/nutreek:demo`, at <https://p01--nutreek--7dcjw6mlmtky.code.run/>.

## How it ships

`.github/workflows/docker-demo.yml` runs on every published release (prereleases are skipped) and by hand (Actions → Docker demo → Run workflow, with a branch, tag or SHA). It:

1. builds the demo image for `linux/amd64` from the release's tag, or the chosen ref;
2. pushes `ghcr.io/easly1989/nutreek:demo` and `:demo-<version>`, where the version is the release's tag (`demo-v2.1.0`) or the commit's short SHA for a run by hand. It never pushes `:latest` or `:vX.Y`;
3. points the Northflank service at `:demo-<version>` with one API call ([update service deployment](https://northflank.com/docs/v1/api/services/update-service-deployment)), `POST /v1/projects/{project}/services/{service}/deployment`. Northflank does not watch ghcr.io by itself, and a new image path is what makes it roll out.

The deploy step is skipped while the `NORTHFLANK_API_TOKEN` secret is missing, so the workflow stays green before Northflank is set up. The workflow is separate from `release.yml`: a broken demo never fails a release.

A release runs the workflows of its tag's commit, so a release whose tag predates `docker-demo.yml` (v2.0.0) does not trigger it: run it by hand once. Every release tagged from `main` since then does.

The service has no volume, so every deploy starts from an empty database and ends every demo in use.

CI tests the demo build on every push: `go test -tags demo ./cmd/nutreek/` builds the binary, starts it on an empty database, enters the demo from the home's form, checks the seeded week and adds a food (`cmd/nutreek/demo_e2e_test.go`). golangci-lint also runs with `--build-tags demo`.

## Northflank setup (once, by hand)

The GHCR package must be public, so Northflank pulls the image without credentials.

**Service.** A deployment service in a project of its own:

- Source: external image `ghcr.io/easly1989/nutreek:demo`, no registry credentials. The workflow replaces it with `:demo-<version>` on every deploy.
- Port: `8080`, HTTP, public.
- No volume on `/data`. One instance is enough.

**Environment variables:**

| Variable | Value |
|---|---|
| `NUTREEK_ENV` | `prod` (secure cookies; Northflank terminates TLS) |
| `NUTREEK_SESSION_KEY` | a Northflank **secret**, made with `openssl rand -hex 32`. It guards nothing worth keeping, but it must not be in the repo |
| `NUTREEK_OFF_SYNC_ENABLED` | `false` (the ~0.8 GB OpenFoodFacts CSV export; search uses the seeded products) |
| `NUTREEK_RECONCILE_ENABLED` | `false` |
| `NUTREEK_BACKUP_ENABLED` | `false` (the demo build switches backups off anyway) |

Leave `NUTREEK_OWNER_EMAIL` and `NUTREEK_ADMIN_TOKEN` unset: the demo build clears them anyway. The `NUTREEK_DEMO_*` limits keep their defaults unless the demo needs others. The image already sets `NUTREEK_ADDR=:8080` and `NUTREEK_DB_PATH=/data/nutreek.db`.

Northflank's proxy connects from a private address, which the rate limits already trust for `X-Forwarded-For`, so the per-address limits count visitors, not the proxy.

**Health checks**, HTTP `GET /healthz` on port 8080 (it pings SQLite, which says more than a TCP check):

| Probe | Settings |
|---|---|
| Readiness | every 10 s, fails after 3 |
| Liveness | initial delay 10 s, every 30 s, fails after 3 |
| Startup | none: the app starts in under a second |

**API token.** A Northflank API token whose role is limited to the demo project, with permission to update the service's deployment.

## GitHub setup (once, by hand)

In the repository's Settings → Secrets and variables → Actions:

| Kind | Name | Value |
|---|---|---|
| Secret | `NORTHFLANK_API_TOKEN` | the API token above |
| Variable | `NORTHFLANK_PROJECT_ID` | the Northflank project's id |
| Variable | `NORTHFLANK_SERVICE_ID` | the demo service's id |

With the token set, a missing project or service id fails the deploy step with a message naming the variables.

Then run the Docker demo workflow by hand once, and check that the demo answers on the service's URL.

## Running it locally

```bash
go run -tags demo ./cmd/nutreek
```

It refuses to start on a database with real users: point `NUTREEK_DB_PATH` at a new file. To try the image:

```bash
docker build --build-arg DEMO=1 -t nutreek:demo .
```

```bash
docker run --rm -p 8080:8080 -e NUTREEK_ENV=dev nutreek:demo
```
