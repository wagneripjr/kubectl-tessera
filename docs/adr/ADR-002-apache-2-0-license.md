---
title: "ADR-002: License under Apache-2.0"
status: Accepted
date: 2026-06-19
authors:
  - Wagner Ignacio Pinto Junior
tags:
  - licensing
  - open-source
supersedes: null
superseded_by: null
---

# ADR-002: License under Apache-2.0

## Status

**Accepted**

## Context

This is an open-source release targeting krew-index. The license affects contribution,
downstream adoption, and patent posture for infrastructure tooling that may be upstreamed.

## Decision

License the project under **Apache-2.0**. It is the Kubernetes-ecosystem norm, krew-index
is overwhelmingly Apache-2.0, and it carries an explicit patent grant — the right default
for infra tooling.

## Consequences

### Positive

- **POS-001**: Matches ecosystem expectations; frictionless krew-index acceptance.
- **POS-002**: Explicit patent grant protects contributors and users.

### Negative

- **NEG-001**: Permissive — does not require downstream modifications to be open-sourced
  (acceptable; this is tooling, not a copyleft-strategic product).

## Alternatives Considered

### Alternative 1: MIT

**Rejected because**: no explicit patent grant; Apache-2.0 is the stronger default for a
credential-minting tool that may be upstreamed.

### Alternative 2: GPL/AGPL

**Rejected because**: copyleft deters adoption and integration in the permissively-licensed
Kubernetes ecosystem; misaligned with krew norms.

## References

- NFR-007 · https://www.apache.org/licenses/LICENSE-2.0
