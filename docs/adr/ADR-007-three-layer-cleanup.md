---
title: "ADR-007: Three-layer cleanup (token TTL + exec trap + gc sweep)"
status: Accepted
date: 2026-06-19
authors:
  - Wagner Ignacio Pinto Junior
tags:
  - lifecycle
  - reliability
supersedes: null
superseded_by: null
---

# ADR-007: Three-layer cleanup (token TTL + exec trap + gc sweep)

## Status

**Accepted**

## Context

A minted session creates RBAC objects and a kubeconfig. Both must be reclaimed even when the
happy path is interrupted (Ctrl-C, crash, `kill -9`, or a non-interactive `--print-kubeconfig`
caller with no process to trap).

## Decision

Defense in depth with three independent layers:

1. **Token TTL** — the API server auto-revokes the token at its `ExpirationTimestamp`. The
   primary protection; needs nothing from us.
2. **Foreground trap (`--exec`)** — on subshell exit or `SIGINT`/`SIGTERM`, delete the
   session's object set and remove the kubeconfig file.
3. **`tessera gc`** — sweep objects by `app.kubernetes.io/managed-by=kubectl-tessera`, parse
   `expires-at`, delete expired sets. Idempotent. Required for `--print-kubeconfig` and for
   recovery after `SIGKILL` bypasses the trap.

## Consequences

### Positive

- **POS-001**: No single failure mode orphans a session indefinitely.
- **POS-002**: `--print-kubeconfig` (agent use) is safe as long as gc runs (host cron or the
  in-cluster CronJob).

### Negative

- **NEG-001**: `SIGKILL` leaves objects until the next gc run — a bounded window, documented.
- **NEG-002**: Teams using `--print-kubeconfig` must operate gc (CronJob shipped in
  `deploy/gc-cronjob.yaml`).

## Alternatives Considered

### Alternative 1: Rely on token TTL alone

**Rejected because**: the token expires but the RBAC objects linger, accumulating clutter and
audit noise.

### Alternative 2: Owner references / TTL controller on the objects

**Rejected because**: no natural owner object exists for the set, and a custom controller is
heavier than a labelled gc sweep; revisit if an operator pattern emerges.

## References

- FR-009, FR-011 · NFR-005 · deploy/gc-cronjob.yaml
