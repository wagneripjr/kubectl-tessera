## What & why

Describe the change and the motivation. Reference the requirement or bug it addresses.

- Requirement / bug: <!-- FR-NNN / NFR-NNN / BUG-NNN -->

## Checklist

- [ ] `go build ./...` and `golangci-lint run` are clean
- [ ] `go test ./...` (unit) passes
- [ ] Acceptance specs pass where applicable: `go test -tags=e2e ./test/e2e/...` (kind)
- [ ] Commit messages are conventional and reference an `(FR|NFR|BUG)-NNN` ID for
      `feat`/`fix`/`perf`
- [ ] `docs/TRACEABILITY.md` updated if requirements/ADRs/bugs changed
- [ ] `CHANGELOG.md` updated under `[Unreleased]`
- [ ] No token/credential material in code, tests, logs, or fixtures
- [ ] Security model unaffected, or the cluster-admin caveat (SECURITY.md) is honored
