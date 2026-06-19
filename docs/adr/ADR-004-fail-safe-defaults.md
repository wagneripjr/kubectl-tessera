---
title: "ADR-004: Fail-safe defaults (verbs, scope, TTL)"
status: Accepted
date: 2026-06-19
authors:
  - Wagner Ignacio Pinto Junior
tags:
  - security
  - ux
supersedes: null
superseded_by: null
---

# ADR-004: Fail-safe defaults (verbs, scope, TTL)

## Status

**Accepted**

## Context

The tool's value is least privilege. Defaults are what most invocations get, so they must
fail safe: a forgotten flag must never silently widen access.

## Decision

Defaults: verbs **`get,list,watch`**; scope **namespaced** (cluster-wide only with an
explicit `--cluster-scoped`); TTL **`15m`**. `--resource` is required (no implicit
all-resources). There is no `--force` to bypass the pre-flight gate.

## Consequences

### Positive

- **POS-001**: The cheap/forgetful path is the safe path — read-only, namespaced, short.
- **POS-002**: Widening is always explicit and visible in the command line / audit log.

### Negative

- **NEG-001**: Common write tasks require explicit verb lists (intended friction).

## Alternatives Considered

### Alternative 1: Default to the operator's full effective permissions

**Rejected because**: defeats the purpose; an omitted flag would hand the agent everything.

### Alternative 2: Provide `--force` to skip pre-flight

**Rejected because**: the gate is a UX safety net; bypassing it adds risk with no real
benefit (the API server still enforces the boundary for non-admins).

## References

- NFR-006 · ADR-006 (pre-flight gate)
