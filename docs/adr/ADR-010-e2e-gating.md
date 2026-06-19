---
title: "ADR-010: Gate the e2e suite by build tag + GODOG_TAGS"
status: Accepted
date: 2026-06-19
authors:
  - Wagner Ignacio Pinto Junior
tags:
  - testing
  - ci
supersedes: null
superseded_by: null
---

# ADR-010: Gate the e2e suite by build tag + GODOG_TAGS

## Status

**Accepted**

## Context

The kind-backed acceptance suite is slow and needs a cluster, while the inner-loop unit tests
must stay fast and dependency-free so `go test ./...` runs on every change.

## Decision

Gate the acceptance suite behind a Go **build tag `e2e`** (everything under `test/e2e` and
the kind-backed driver carries `//go:build e2e`), and sub-select scenarios at runtime via the
**`GODOG_TAGS`** env (e.g. `"@e2e && ~@manual"`). This yields three tiers:

```
go test ./...                                  # unit, no cluster, fast — every PR
go test -tags=e2e ./test/e2e/...               # full acceptance on kind — dedicated CI job
GODOG_TAGS="@preflight" go test -tags=e2e ...  # focused local runs
```

## Consequences

### Positive

- **POS-001**: `go test ./...` never accidentally pulls in cluster dependencies.
- **POS-002**: CI separates a fast unit gate from the kind e2e job; tags exclude `@manual`.

### Negative

- **NEG-001**: Contributors must remember `-tags=e2e` to run acceptance locally (documented
  in CONTRIBUTING.md).

## Alternatives Considered

### Alternative 1: A single suite that detects a cluster at runtime and skips

**Rejected because**: muddies the unit/acceptance boundary and risks silent skips that read
as green.

### Alternative 2: Separate test binary built by the `godog` CLI

**Rejected because**: adds a tool dependency; `go test` integration via `Options.TestingT` is
simpler and CI-friendly.

## References

- ADR-009 · .github/workflows/ci.yaml
