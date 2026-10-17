# Contributing to Nutreek

Thanks for wanting to help. Nutreek is open source (AGPL-3.0) and built by one
person in their spare time, so small, focused contributions are the easiest to
review.

## Translating the interface

The easiest way in, and no code is needed: adding a language is a few JSON files
and one line in a list. Follow the [translation guide](docs/translating.md) and
the [glossary](docs/glossary.md). Open an issue with the **New language**
template first if you want to claim a language.

## Code

1. **Open or pick an issue** before a large change, so we agree on the approach.
   One issue, one branch, one pull request to `main`; never push to `main`.
2. **Set up**: you need Go 1.27 or later. `go run ./cmd/nutreek` starts the app on
   <http://localhost:8080>; the stylesheet is generated, see
   [Try it locally](docs/translating.md#try-it-locally) or the Quickstart in the
   [README](README.md). The layout of the code is in
   [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).
3. **Branch** with a prefix: `feat/`, `fix/`, `chore/`, `ci/`, `refactor/`,
   `test/`, `docs/`.
4. **Commit** in English, [Conventional Commits](https://www.conventionalcommits.org/)
   with a scope: `feat(shopping): add past weeks dropdown`.
5. **Everything is in English**: code, comments, tests, commit messages, issues,
   pull requests. Text the app shows goes into the translation catalogs
   (`internal/i18n/locales`), in Italian and English together, never straight into
   a template or a handler.
6. **Check before you push** (CI runs the same):

   ```
   go build ./...
   go vet ./...
   gofmt -l .
   go test ./...
   ```

   `gofmt -l .` must print nothing. CI also runs `golangci-lint` (configuration in
   `.golangci.yml`).
7. **Open the pull request** to `main` with `Closes #N` in the description. If it
   changes something people see, add real screenshots (desktop and phone), using
   demo data only.

Add a line to `CHANGELOG.md` under *Unreleased* for anything users will notice.

## Bugs and ideas

Open an issue. For a bug: what you did, what you expected, what happened, and the
version (`nutreek -version`). For security problems, do not open a public issue;
write to the maintainer through the contact on the GitHub profile.

## License

By contributing you agree that your work is released under the
[AGPL-3.0](LICENSE), like the rest of the project.
