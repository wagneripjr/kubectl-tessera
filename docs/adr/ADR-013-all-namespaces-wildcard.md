---
title: "ADR-013: All-namespaces wildcard via a single ClusterRoleBinding"
status: Accepted
date: 2026-06-22
authors:
  - Wagner Ignacio Pinto Junior
tags:
  - security
  - authorization
  - scope
supersedes: null
superseded_by: null
---

# ADR-013: All-namespaces wildcard via a single ClusterRoleBinding

## Status

**Accepted**

## Context

FR-017 lets an operator name an explicit list of namespaces. Operators also need the
kubectl `-A` ergonomic: "this scope, in every namespace." Naïvely that could be implemented
by enumerating the namespaces that exist at mint time and creating one Role+RoleBinding per
namespace (FR-017's mechanism). That has three problems: it does not cover namespaces created
after the mint; the object count and the SSAR pre-flight scale with cluster size; and a new
namespace silently has no access, which is surprising for a grant the operator believes is
"all namespaces."

The only Kubernetes construct that grants a namespaced resource (e.g. `pods`) across every
namespace, including future ones, is a **ClusterRole bound by a ClusterRoleBinding**. That is
the same object machinery `--cluster-scoped` already uses, but the intent differs:
`--cluster-scoped` targets cluster-scoped resource *types* (nodes, PVs); the all-namespaces
wildcard targets *namespaced* types in every namespace. This is the widest credential tessera
can mint and is the opposite of the tool's narrow-by-default thesis (NFR-006), so it must be
opt-in, gated precisely, and audited loudly.

## Decision

Add `-A`/`--all-namespaces` (with `-n '*'` accepted as sugar). It mints **one** ServiceAccount
+ **one** ClusterRole + **one** ClusterRoleBinding carrying the requested namespaced-resource
rules. Specifically:

- **Cluster-wide SSAR gate.** The pre-flight checks each requested rule with an **empty
  namespace** (`auth can-i <verb> <resource>` cluster-wide). Combined with create-as-user
  (ADR-005) and the API server's RBAC escalation-prevention, an operator can only mint a
  wildcard they already hold cluster-wide — the grant is self-limiting.
- **Never a default** (NFR-006). The wildcard is reached only via the explicit flag/sentinel.
- **Loud audit.** A cluster-wide warning is written to stderr, and the audit line records
  `ns=*`.
- **Mutually exclusive** with `--cluster-scoped` and with an explicit `-n` list. A
  cluster-scoped resource *type* under `-A` is rejected with a pointer to `--cluster-scoped`.
- **Reuses the cluster-scoped object path** in `internal/rbac` unchanged — the rbac package is
  indifferent to whether the rules name namespaced or cluster-scoped resources.

## Consequences

### Positive

- **POS-001**: Future namespaces are covered automatically; O(1) objects and SSAR calls
  regardless of cluster size.
- **POS-002**: No new privileged path — the cluster-wide objects are created as the invoking
  user, so the escalation-prevention boundary (NFR-002, ADR-005) still holds.
- **POS-003**: `gc`/`ls` already sweep cluster-scoped kinds by label, so the wildcard session
  is reclaimed and listed with no cleanup changes.

### Negative

- **NEG-001**: The wildcard is genuinely broad; a mis-issued one grants cluster-wide read. The
  SSAR gate and loud audit mitigate, but the blast radius is real — hence opt-in only.
- **NEG-002**: A `--print-kubeconfig` wildcard leaves a ClusterRoleBinding for `gc`; until it
  expires it is a cluster-wide object (same property as `--cluster-scoped`).

## Alternatives Considered

### Alternative 1: Enumerate current namespaces into per-namespace Role+RoleBinding sets

**Rejected because**: it does not cover namespaces created after the mint, scales objects and
SSAR calls with cluster size, and gives a false sense of "all namespaces."

### Alternative 2: Fold the wildcard into FR-017's requirement and the `--namespace` flag only

**Rejected because**: the wildcard is a distinct, wider security posture (cluster-wide binding
vs per-namespace bindings); conflating them under one requirement obscures the SSAR-gate
difference. It gets its own FR-018 + this ADR. `-n '*'` is still accepted as sugar for
operators who reach for the wildcard syntax.

## References

- FR-018 · NFR-002, NFR-006 · ADR-004 (fail-safe defaults) · ADR-005 (create-as-user) ·
  ADR-006 (SSAR gate) · ADR-008 (labels) · `kubectl auth can-i`
