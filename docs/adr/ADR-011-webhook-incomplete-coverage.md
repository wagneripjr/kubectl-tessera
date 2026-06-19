---
title: "ADR-011: Cover SSRR Incomplete with a unit surrogate + manual e2e"
status: Accepted
date: 2026-06-19
authors:
  - Wagner Ignacio Pinto Junior
tags:
  - testing
  - authorization
supersedes: null
superseded_by: null
---

# ADR-011: Cover SSRR Incomplete with a unit surrogate + manual e2e

## Status

**Accepted**

## Context

FR-013 requires surfacing the SSRR `Incomplete` flag. `Incomplete` is only truthfully set by a
**non-enumerable authorizer** (e.g. a webhook). Reproducing that on kind needs a dedicated
cluster configured with `--authorization-mode=…,Webhook` plus a webhook service — heavyweight
and brittle for standard CI.

## Decision

Two-track coverage:

1. **Unit surrogate (automated, every PR):** unit-test the *notice-rendering* path by faking an
   SSRR response with `Status.Incomplete = true` and asserting the stderr notice matches
   `(?i)discovery may be incomplete`. This covers the user-facing behavior deterministically.
2. **Manual e2e (`@manual @webhook`):** keep the genuine non-enumerable-authorizer scenario
   written in `discovery.feature` but tagged so it is excluded from standard CI; document the
   webhook-authz kind setup for on-demand runs.

This is the one acceptance criterion (#11) that cannot be fully automated in the standard kind
job; the limitation is explicit, not silent.

## Consequences

### Positive

- **POS-001**: Real automated coverage of the notice without a brittle webhook cluster.
- **POS-002**: The faithful scenario still exists as an executable spec for manual runs.

### Negative

- **NEG-001**: CI does not exercise the genuine non-enumerable path; relies on the unit
  surrogate + manual verification.

## Alternatives Considered

### Alternative 1: Stand up a webhook-authz kind cluster in CI

**Rejected because**: high complexity/flake for one notice; poor cost/benefit.

### Alternative 2: Drop automated coverage of #11 entirely

**Rejected because**: the user-facing notice is testable cheaply at the unit level; skipping it
would leave a real behavior unverified.

## References

- FR-013 · ADR-006 · ADR-009/010
