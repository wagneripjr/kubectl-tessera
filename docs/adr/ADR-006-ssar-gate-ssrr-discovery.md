---
title: "ADR-006: SSAR is the authorization gate; SSRR is discovery only"
status: Accepted
date: 2026-06-19
authors:
  - Wagner Ignacio Pinto Junior
tags:
  - security
  - authorization
supersedes: null
superseded_by: null
---

# ADR-006: SSAR is the authorization gate; SSRR is discovery only

## Status

**Accepted**

## Context

Two authorization-review APIs exist. `SelfSubjectAccessReview` (SSAR) answers a precise
yes/no for one `(verb, resource, namespace[, name])` — equivalent to `kubectl auth can-i`.
`SelfSubjectRulesReview` (SSRR) enumerates "what could I do here" — equivalent to
`kubectl auth can-i --list` — but its result can be **incomplete** for non-enumerable
authorizers (e.g. webhook) and it does **not** evaluate `resourceNames`. Confusing the two
would let an over-ask slip past the gate.

## Decision

Use **SSAR as the authoritative pre-flight gate**: one review per requested rule (including
`name` scope); any denial aborts before creating anything (FR-003). Use **SSRR for
discovery only** (`--dry-run`/`--explain`), and always surface `.Status.Incomplete` as a
"discovery may be incomplete" notice (FR-013). SSRR is never used to authorize.

## Consequences

### Positive

- **POS-001**: The gate is precise and honors `resourceNames`, matching what gets created.
- **POS-002**: Discovery is helpful without being mistaken for a guarantee.

### Negative

- **NEG-001**: One SSAR per `(verb × resource × ns × name)` can be many API calls for broad
  asks (acceptable; bounded by the request size).
- **NEG-002**: Faithfully testing the SSRR `Incomplete` path needs a non-enumerable
  authorizer (see ADR-011).

## Alternatives Considered

### Alternative 1: Gate on SSRR (`can-i --list`) once, locally

**Rejected because**: SSRR can be incomplete and ignores `resourceNames` — it would
authorize asks the API server would later refuse, or miss name-scoped denials.

### Alternative 2: Skip the gate; rely solely on API-server enforcement at binding creation

**Rejected because**: the gate gives a fast, clear allowed/denied table before any object
exists; relying only on creation failure yields worse UX and partial-creation risk.

## References

- FR-003, FR-013 · ADR-011 · `kubectl auth can-i`
