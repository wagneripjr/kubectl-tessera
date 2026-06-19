# Architecture Boundaries — kubectl-tessera

Machine-readable import-direction rules for the commit-time architecture-boundary gate. The
design is a layered CLI: `internal/cli` orchestrates single-responsibility packages, which
depend only downward, with `internal/labels` as a shared leaf.

## Module

`github.com/wagneripjr/kubectl-tessera`

## Layers (top → bottom)

1. `cmd/kubectl-tessera` — entrypoint. Imports only `internal/cli`.
2. `internal/cli` — Cobra commands + flag wiring (orchestration).
3. `internal/{scope,preflight,rbac,token,kubeconfig,gc}` — single-responsibility units.
4. `internal/labels`, `internal/version` — shared leaves (no internal deps).

## Allowed import directions

- `cmd/kubectl-tessera` → `internal/cli` only.
- `internal/cli` → any `internal/*`.
- `internal/{scope,preflight,rbac,token,kubeconfig,gc}` → `internal/labels`, `internal/version`,
  and external libs (client-go, apimachinery, cli-runtime). They may **not** import each other
  except through `internal/cli` orchestration (no lateral coupling between units).
- `internal/labels`, `internal/version` → standard library only.

## Forbidden (gate denies)

- Any `internal/*` package importing `internal/cli` (no upward imports).
- Any production package (`cmd/`, `internal/`) importing `test/*` or godog.
- **NFR-002 enforcement:** no production package may construct a Kubernetes client from
  anything other than the resolved `ConfigFlags` identity. Specifically, no use of
  `rest.Config.Impersonate`, no `--as`/`--as-group` wiring, and no alternate/privileged
  kubeconfig path for object creation. Object creation runs as the invoking user only.

## Test layers (not subject to the layer rule, but isolated)

- `specs/features/*.feature` — Gherkin, no Go imports.
- `test/dsl`, `test/drivers`, `test/e2e` — may import client-go and the compiled binary path;
  must **not** import `internal/*` (Go's internal rule enforces this, keeping the acceptance
  suite black-box — Gate G4 of ATDD).
