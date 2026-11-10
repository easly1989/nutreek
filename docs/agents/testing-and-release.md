# Testing, CI, release and demo

Moved from CLAUDE.md. Read when writing tests or touching workflows, releases or the demo.

## Testing

- `go test ./...` covers: auth, macros, the db layer (every repository), OFF client/reconciler, rate limiter, API tokens, features
- Pattern: table tests, `t.Cleanup()` for resources, a temporary SQLite DB for isolation
- GitHub Actions CI (`.github/workflows/ci.yml`): tests with `-race`, golangci-lint (`.golangci.yml`), binary and Docker image build, govulncheck
- Release (`.github/workflows/release.yml`): publishing a GitHub Release `vX.Y.Z` builds and pushes the public image `ghcr.io/easly1989/nutreek` tagged `:latest`, `:vX.Y.Z`, `:vX.Y`
- Demo (`.github/workflows/docker-demo.yml`): every published release (not prereleases) and a run by hand push `ghcr.io/easly1989/nutreek:demo` and `:demo-<version>`, then deploy it on Northflank. CI runs `go test -tags demo ./cmd/nutreek/`, an end-to-end test of the demo binary. Setup and secrets: [`docs/demo.md`](../demo.md)
- Manual UI testing: `make dev` → `localhost:8080`

## Branch workflow

One issue → one branch → one PR to `main`, with `Closes #N` in the description.

1. A dedicated branch with a `feat/ fix/ chore/ ci/ refactor/ test/ docs/` prefix (e.g. `feat/shopping-past-weeks`)
2. Atomic commits, Conventional Commits in English
3. Push: `git push -u origin <branch>`, then a PR to `main`; CI must be green
4. Issues and PRs that touch the UI carry images (a mockup in the issue, real screenshots in the PR)
5. **Never push directly to `main`**
