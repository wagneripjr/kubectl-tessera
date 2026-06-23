---
title: "ADR-014: Explicit all-resources wildcard via a single `*/*` rule"
status: Accepted
date: 2026-06-23
authors:
  - Wagner Ignacio Pinto Junior
tags:
  - security
  - authorization
  - scope
supersedes: null
superseded_by: null
---

# ADR-014: Explicit all-resources wildcard via a single `*/*` rule

## Status

**Accepted**

## Context

FR-002 resolves every requested `--resource` through a discovery-backed RESTMapper. A literal `*`
is not a registered resource, so `--resource '*'` fails with `unknown resource "*"`. Operators
nevertheless need an all-resources session — most often the *read-only agent* case the README
advertises ("point an AI agent at a cluster read-only for one hour"), which wants
`{apiGroups:["*"], resources:["*"], verbs:[get,list,watch]}` rather than an enumerated list that
must be kept in sync with the cluster's installed CRDs.

This is the same posture tension as ADR-013's `-A`: the widest credential tessera can mint, the
opposite of the narrow-by-default thesis (NFR-006). NFR-006, however, forbids only *implicit*
widening — an explicit `--resource '*'` opt-in leaves every default narrow. The deciding property
is that the existing SSAR pre-flight already gates it: a `SelfSubjectAccessReview` for verb on
`*`/`*` is the canonical "am I admin?" check, so a non-admin operator is refused with no special
handling and the escalation-prevention boundary (NFR-002, ADR-005) holds for free.

## Decision

Accept `--resource '*'` as an explicit all-resources request. `scope.Resolve` **short-circuits**
when the request is exactly `["*"]`, returning a single resolved resource (`Group:"*"`,
`Resource:"*"`, `Namespaced: !ClusterScoped`) and one rule
`{APIGroups:["*"], Resources:["*"], Verbs, ResourceNames}`. The RESTMapper is not consulted.

- **Downstream is unchanged.** `buildGrantAttributes`→SSAR, `internal/rbac` creation, the audit
  line, and the dry-run descriptor are all generic over `ResolvedResource`/`PolicyRule`, so the
  wildcard flows through every existing path with no new branches. The existing
  `clusterWide := --cluster-scoped || -A` still picks Role vs ClusterRole.
- **SSAR gate is the boundary.** The pre-flight checks `*/*` (cluster-wide when clusterWide, else in
  each session namespace). A non-admin is refused before any object is created — self-limiting, same
  as ADR-013.
- **Never a default** (NFR-006). Reached only via the explicit `*` sentinel.
- **Loud audit.** A stderr warning records that the session grants all resources (admin-equivalent
  for the chosen verbs); the audit line already records the scope.
- **Rejections.** `*` mixed with other resources (`pods,*`) — like the namespace-wildcard
  rejection; `*` with `--resource-name` (naming all resources is nonsensical); `*` with
  `--api-group` (the wildcard already spans all groups).

## Consequences

### Positive

- **POS-001**: One short-circuit; no change to preflight, rbac, gc, ls, or kubeconfig code — the
  blast radius of the change is a single `Resolve` branch plus flag validation.
- **POS-002**: No new privileged path — the `*/*` objects are created as the invoking user, so the
  escalation-prevention boundary (NFR-002, ADR-005) still holds and gates non-admins automatically.
- **POS-003**: Serves the README's own read-only-agent use case directly, without enumerating CRDs.

### Negative

- **NEG-001**: `--resource '*' --verb '*'` is an ephemeral admin-equivalent token; broad by
  construction. The SSAR gate (admins only) and loud audit mitigate, but blast radius is real —
  hence opt-in.
- **NEG-002**: A `--print-kubeconfig` wildcard leaves a Role/ClusterRole+binding for `gc`; until it
  expires it is a broad object (same property as `-A`/`--cluster-scoped`).

## Alternatives Considered

### Alternative 1: Expand `*` to the live discovery resource list at mint time

**Rejected because**: it does not cover CRDs/resources installed after the mint, scales the rule and
SSAR calls with the cluster's API surface, and gives a false sense of "all resources." A single
`{*,*}` rule is what Kubernetes itself uses for admin.

### Alternative 2: A dedicated `--all-resources` flag instead of the `*` sentinel

**Rejected because**: `*` mirrors the existing namespace wildcard (`-n '*'`) and standard RBAC
syntax; a separate flag adds surface without changing semantics. The `*` short-circuit is the
smallest change.

## References

- FR-019 · NFR-002, NFR-006 · ADR-004 (fail-safe defaults) · ADR-005 (create-as-user) ·
  ADR-006 (SSAR gate) · ADR-013 (all-namespaces wildcard) · `kubectl auth can-i '*' '*'`
