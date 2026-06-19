---
title: "ADR-008: Label/annotation schema with shared session-id"
status: Accepted
date: 2026-06-19
authors:
  - Wagner Ignacio Pinto Junior
tags:
  - conventions
  - lifecycle
supersedes: null
superseded_by: null
---

# ADR-008: Label/annotation schema with shared session-id

## Status

**Accepted**

## Context

`gc`, `ls`, and cleanup must identify and operate on the objects of a single mint as a unit,
across namespaces, without a parent object — and must never touch unmanaged RBAC.

## Decision

Stamp every managed object with these **labels**:

- `app.kubernetes.io/managed-by: kubectl-tessera`
- `tessera.adustio.com/owner: <sanitized-owner>`
- `tessera.adustio.com/session-id: <sessionID>`

and this **annotation**:

- `tessera.adustio.com/expires-at: <RFC3339-UTC>`

The `managed-by` label is the safety selector (only ever touch our objects). The shared
`session-id` makes a whole session (including multi-namespace sets, FR-017) addressable
atomically. `expires-at` (UTC RFC3339) drives gc. The `tessera.adustio.com` domain is the
project's owned DNS subdomain — keys are defined once in `internal/labels`.

## Consequences

### Positive

- **POS-001**: gc/ls/cleanup are simple label-selector operations with a hard safety filter.
- **POS-002**: Multi-namespace and cluster-scoped objects of one session are reclaimed together.

### Negative

- **NEG-001**: The label domain is baked into specs, gc selectors, and code — changing it
  later is a coordinated edit (locked early; see project CLAUDE.md).

## Alternatives Considered

### Alternative 1: OwnerReferences to a parent ConfigMap

**Rejected because**: adds an object to manage and doesn't span cluster-scoped objects cleanly;
labels are sufficient and simpler.

### Alternative 2: Encode metadata only in object names

**Rejected because**: names are length-limited (DNS-1123) and can't carry an annotation
timestamp or be selected on efficiently.

## References

- FR-004, FR-011, FR-012, FR-017 · internal/labels
