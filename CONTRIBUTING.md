# Contributing to kubectl-tessera

Thanks for considering a contribution. This is a pre-1.0 project; expect things to move and break.

## Default branch

The default/base branch is **`master`**. Branch off `master`, target your PRs at `master`.

## Build, test, lint

```bash
# Build everything
go build ./...

# Unit tests
go test ./...

# Acceptance (ATDD) tests against a kind cluster — needs a running kind cluster on your KUBECONFIG
go test -tags=e2e ./test/e2e/...

# Lint
golangci-lint run
```

The acceptance suite mints real RBAC objects and tokens, so it needs a real cluster. A local
[kind](https://kind.sigs.k8s.io/) cluster is the expected target; the `e2e` build tag keeps those
tests out of the default `go test ./...` run.

## Testing approach: ATDD

This project uses **Acceptance Test-Driven Development** with [godog](https://github.com/cucumber/godog).

- Acceptance specs live in `specs/features/` as `.feature` files, written in business language with
  zero implementation detail.
- The tool's external surface (the CLI) is exercised through a **protocol driver** — tests drive
  `kubectl tessera` the way a user would, never by calling internals.
- Production code follows from a failing acceptance test (RED), then the inner unit-TDD loop
  (RED → GREEN → REFACTOR).

If you're adding a feature, start from a `.feature` scenario, not from `src/`.

## Commits

Use **conventional commits**, and reference the requirement or bug ID for code-changing commits:

```
feat(FR-001): add SSAR pre-flight gate
fix: reclaim orphaned bindings on SIGKILL  BUG-003
perf(NFR-002): batch the token request
```

- `feat:` / `fix:` / `feat!:` / `perf:` **require** an `FR-NNN`, `NFR-NNN`, or `BUG-NNN` ID.
- `docs:` / `chore:` / `ci:` / `test:` / `style:` / `refactor:` / `build:` / `revert:` are **exempt**.

Commit messages explain **why**, not what. The diff already shows what.

## DCO / sign-off

Sign-off is **optional** but welcome. If you want to certify the
[Developer Certificate of Origin](https://developercertificate.org/), add `-s`:

```bash
git commit -s -m "feat(FR-001): ..."
```

## Pull requests

Before opening a PR, make sure: `go build ./...` is clean, `go test ./...` passes,
`golangci-lint run` is clean, and the PR checklist (see the PR template) is satisfied. If your
change touches requirements, update `docs/TRACEABILITY.md` in the same PR.
