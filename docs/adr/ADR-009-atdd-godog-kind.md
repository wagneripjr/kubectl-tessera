---
title: "ADR-009: ATDD via godog on kind with a composite protocol driver"
status: Accepted
date: 2026-06-19
authors:
  - Wagner Ignacio Pinto Junior
tags:
  - testing
  - atdd
supersedes: null
superseded_by: null
---

# ADR-009: ATDD via godog on kind with a composite protocol driver

## Status

**Accepted**

## Context

Tessera's correctness is not observable from the binary's own output — the load-bearing
claim is that the *minted token* grants exactly the requested verbs and nothing more, which
only the API server can adjudicate. Trusting the binary's stdout would be circular. The
faithful system-under-test is therefore the compiled binary **plus a real API server**.

## Decision

Adopt **ATDD** with **godog** (Cucumber for Go). The acceptance suite drives the SUT through
its two real external protocols via a **composite protocol driver** (`test/drivers`):

- a **process adapter** that spawns the compiled `kubectl-tessera` binary and captures
  stdout/stderr/exit code; and
- a **cluster adapter** (client-go) that verifies object state and, crucially, runs SSAR
  *with the minted token* (`auth can-i`) and issues real requests with it to prove effective
  authorization and TTL expiry.

The real API server is **kind**. Specs live in `specs/features` (business language, QA-owned);
DSL + driver live in `test/` (Dev-owned). The acceptance suite reaches the SUT only through
the binary and the API server — never `internal/` (enforced by Go's internal-package rule).

## Consequences

### Positive

- **POS-001**: Tests prove real authorization, not the binary's self-report.
- **POS-002**: Clean 4-layer ATDD separation; specs are implementation-agnostic.

### Negative

- **NEG-001**: Acceptance tests need a kind cluster — slower and CI-infrastructure-dependent
  (mitigated by ADR-010 gating and namespace-per-scenario isolation).

## Alternatives Considered

### Alternative 1: Mock the API server with `client-go/fake`

**Rejected because**: fake clients don't enforce RBAC/escalation-prevention or mint real
tokens — they would test our assumptions, not Kubernetes' behavior. (Used for inner-loop
unit tests only.)

### Alternative 2: `go test` assertions calling internal functions directly

**Rejected because**: bypasses the external protocol; couples tests to internals; can't prove
the minted credential works end to end.

## References

- ADR-010 · `wagner-skills:atdd` · docs/design/protocol-drivers.md
