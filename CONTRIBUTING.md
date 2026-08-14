# Contributing to PleumCloud

Thanks for helping build one drive for all the free cloud storage! 🌥️

## Ways to help

- **New cloud connectors** — the highest-value contributions. Every
  connector lives in `internal/provider/<name>/` behind one interface
  (see `internal/provider/provider.go`). Bring an API that isn't
  supported yet.
- **Bug reports & fixes** — include your OS, the provider, and what you
  expected. Never paste real tokens into issues.
- **Translations** — add or improve a README translation under `readme/`,
  or UI strings (i18n is landing soon).
- **Docs** — provider research, setup guides, comparisons.

## Adding a connector (TDD)

1. **Pin the API first** from a primary source (official docs, an OpenAPI
   spec, or a maintained client library). No guessing endpoints.
2. Write the failing tests in `internal/provider/<name>/<name>_test.go`
   using `httptest` mocks (see `gdrive` or `mybox` for the pattern — every
   connector's base URL is a package var for exactly this reason).
3. Implement the `Connector` interface and register the factory in
   `init()`. Add the provider to the catalog with honest free-tier data.
4. Add a logo to `web/public/logos/<id>.svg` (official asset when a
   license permits; a brand-colored tile otherwise) and document its
   source in `web/public/logos/README.md`.
5. `go test ./...` green → open the PR.

## Ground rules

- **Official APIs only.** No reverse-engineered endpoints for providers
  without public APIs — user account safety beats coverage.
- Tests for pure logic; mock-based tests for protocol code.
- Keep the single-binary, no-runtime-deps promise.
- Be kind. Issues and reviews stay on-topic and respectful.

## Development setup

```bash
# Go 1.26+, Node 20+
make build   # builds web/ then embeds it into ./pleumcloud
./pleumcloud # http://localhost:7777

go test ./internal/...   # full suite
```

For OAuth connectors you'll want your own app keys — see
[docs/oauth-setup.md](docs/oauth-setup.md).

## License

By contributing you agree your work is licensed under the project's
AGPL-3.0 (and, if a CLA is introduced later, under its terms).
