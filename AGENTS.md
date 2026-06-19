# AGENTS.md

Open-standard agent instructions for `kubectl-tessera`. This file is a pointer; the
authoritative instructions live in the CLAUDE.md files below (precedence: project overrides
global).

## Instruction sources (in precedence order)

1. **Global:** `~/.claude/CLAUDE.md` — Wagner's environment-wide engineering standards
   (SDLC phases, ATDD, traceability, commit conventions, default branch `master`).
2. **Project:** `./CLAUDE.md` — `kubectl-tessera` invariants: locked parameters (module,
   label domain `tessera.adustio.com`, krew name), the Hard "do NOT" list (create-as-user,
   SSAR-not-SSRR gate, no orphans, no wide default, no token leakage), the `--cluster-scoped`
   flag naming, and ATDD-with-godog testing.

## Quick orientation

- Design of record: `docs/plans/kubectl-tessera-implementation-plan.md`.
- Requirements: `docs/requirements/` (FR/NFR). Decisions: `docs/adr/`. Map:
  `docs/TRACEABILITY.md`. Glossary: `docs/DEFINITIONS.md`. Boundaries: `docs/BOUNDARIES.md`.
- Build: `go build ./...`. Unit: `go test ./...`. Acceptance: `go test -tags=e2e ./test/e2e/...`
  (needs a kind cluster). Lint: `golangci-lint run`.

CLAUDE.md is the source of truth. Keep this file and GEMINI.md in sync with it.
