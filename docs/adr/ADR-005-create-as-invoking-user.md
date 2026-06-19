---
title: "ADR-005: Create RBAC objects as the invoking user — no impersonation"
status: Accepted
date: 2026-06-19
authors:
  - Wagner Ignacio Pinto Junior
tags:
  - security
  - architecture
supersedes: null
superseded_by: null
---

# ADR-005: Create RBAC objects as the invoking user — no impersonation

## Status

**Accepted**

## Context

The tool's claim of being a *real* security boundary for non-admin operators depends
entirely on Kubernetes RBAC escalation-prevention: the API server refuses to let a user
create a binding to a role whose rules they don't already hold (unless they have `bind`).
That check only fires when the creating identity *is* the operator. If the tool created
objects via a privileged context or impersonation, escalation-prevention would be bypassed
and the boundary would be fictional.

## Decision

All RBAC objects (SA, Role/ClusterRole, RoleBinding/ClusterRoleBinding) are created using
the **invoking user's own resolved credentials** (`ConfigFlags`). The tool must **never**
switch to a privileged context and **never** use impersonation (`--as`) to create objects.
This is an architectural constraint enforced by `docs/BOUNDARIES.md` and code review, not
by a black-box test (a black-box test cannot prove the absence of a hidden privileged
path).

## Consequences

### Positive

- **POS-001**: Non-admin operators get a genuine, API-server-enforced privilege ceiling.
- **POS-002**: Rollback and cleanup also run as the operator — no privileged residue.

### Negative

- **NEG-001**: Operators lacking `create` on SA/role/rolebinding get a hard failure (by
  design — surfaced clearly per FR-016).
- **NEG-002**: For cluster-admin operators the boundary is vacuous; this must be stated
  plainly (see VISION + README) and never contradicted.

## Alternatives Considered

### Alternative 1: Use a privileged service identity to create objects

**Rejected because**: it destroys the non-admin security property entirely — the whole
point of the tool.

### Alternative 2: Impersonate the user from an admin context

**Rejected because**: impersonation is itself a privileged capability; using it would both
require admin and bypass the escalation check semantics we rely on.

## References

- NFR-002 · VISION.md (cluster-admin caveat) · docs/BOUNDARIES.md
