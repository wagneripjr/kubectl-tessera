# Requirements — Minting Core

Functional requirements for the core `mint` flow: scope resolution, the authorization
gate, RBAC object creation, and token issuance. See `docs/TRACEABILITY.md` for the
forward map to ADRs, feature files, and tests.

## FR-001: Mint an ephemeral scoped credential

The root command (`kubectl tessera`) mints an ephemeral, scope-narrowed, TTL-bound
credential for the current cluster and emits a throwaway kubeconfig referencing it.

- **Acceptance:** given a valid scope, a kubeconfig is produced whose token grants exactly
  the requested verbs on the requested resources/namespaces and nothing more.
- **Defaults (fail-safe, NFR-006):** verbs `get,list,watch`; namespaced (not cluster-wide);
  TTL `15m`.
- **Traces to:** ADR-001, ADR-004 · `scope_enforcement.feature` (#1).

## FR-002: Resolve scope via discovery RESTMapper

For each requested resource, resolve GVR/GVK and namespaced-vs-cluster-scoped via a
discovery-backed RESTMapper.

- Reject `--namespace` combined with a cluster-scoped resource; instruct the user to use
  `--cluster-scoped`.
- Require `--api-group` only when a resource name is ambiguous across API groups.
- **Acceptance:** namespaced/cluster mismatch and cross-group ambiguity both fail clearly
  before any object is created.
- **Traces to:** ADR-001 · `scope_enforcement.feature`, unit tests (parse/mapping).

> **Flag-naming deviation from the handoff plan:** the implementation plan used `--cluster`
> for "cluster-scoped resources", but `k8s.io/cli-runtime` `ConfigFlags` already registers a
> global `--cluster` (the kubeconfig cluster name). Because ADR-001 commits us to resolving
> config exactly the way kubectl does, tessera's flag is named **`--cluster-scoped`** to
> avoid the collision. The kubeconfig-cluster `--cluster` retains its kubectl meaning.

## FR-003: SelfSubjectAccessReview pre-flight gate

For every `(verb × resource × namespace[ × name])` combination, create a
`SelfSubjectAccessReview` and check `.Status.Allowed`. On any denial, print an
allowed/denied table and exit non-zero **before creating anything**. There is no
`--force`.

- **Acceptance:** a request for a verb the operator lacks aborts pre-creation, prints the
  table, and returns a non-zero exit code. No managed objects exist afterward.
- **Traces to:** ADR-006 · `preflight_gate.feature` (#2).

## FR-004: Create the managed RBAC object set as the invoking user

Create, in order, `ServiceAccount` → `Role`/`ClusterRole` → `RoleBinding`/`ClusterRoleBinding`
via the typed clientset, **as the invoking user** (never a privileged context, never
impersonation — see NFR-002). Stamp every object with the label/annotation schema
(ADR-008).

- **Acceptance:** for a namespaced scope, an SA + Role + RoleBinding appear in the target
  namespace, each carrying `app.kubernetes.io/managed-by=kubectl-tessera`,
  `tessera.adustio.com/owner`, `tessera.adustio.com/session-id`, and the
  `tessera.adustio.com/expires-at` annotation.
- **Traces to:** ADR-005, ADR-008 · `scope_enforcement.feature`.

## FR-005: Reverse-order rollback on partial failure

Track created objects; on any error mid-creation, delete them in reverse order
(foreground propagation) and return. Never leave orphans.

- **Acceptance:** an injected mid-creation failure leaves zero managed objects for that
  session.
- **Traces to:** ADR-005 · `lifecycle_cleanup.feature` (#9).

## FR-006: Mint the token via the TokenRequest API

Mint the ServiceAccount token through `CoreV1().ServiceAccounts(ns).CreateToken(...)` with
`ExpirationSeconds` from the requested TTL, honoring the cluster's bounds in **both**
directions:

- **Floor (below the minimum):** the kube-apiserver hard-rejects any `ExpirationSeconds`
  below its minimum (10 minutes, a non-configurable, non-discoverable `ValidateTokenRequest`
  constant). Rather than fail, floor a sub-minimum requested TTL up to that minimum before
  the request, and warn to stderr that the lifetime was floored.
- **Clamp (above the maximum):** use the **returned** `ExpirationTimestamp` (which reflects
  clamping by `--service-account-max-token-expiration`); if it is shorter than requested,
  warn to stderr.

The effective expiry surfaced to the user is always the API server's returned timestamp.

- **Acceptance:** a TTL below the cluster minimum is floored to it, the credential works,
  and a floor warning is emitted to stderr; a TTL above the cluster maximum surfaces the
  returned (clamped) timestamp and emits a clamp warning.
- **Traces to:** ADR-001 · `lifecycle_cleanup.feature` (#3, sub-minimum TTL floor).

## FR-019: Explicit all-resources scope (`--resource '*'`)

An operator may request **all resources** in one session by passing `--resource '*'`. This is an
explicit, opt-in widening — never a default (NFR-006). The minted rule is
`{apiGroups:["*"], resources:["*"], verbs:[…]}`; the namespacing flags decide Role vs ClusterRole
exactly as for a named resource (default → namespaced `Role`; `-A` → all-namespaces ClusterRole;
`--cluster-scoped` → cluster-wide ClusterRole). Verbs still default to `get,list,watch`, so the
common form `--resource '*' --verb get,list,watch -A` is an ephemeral cluster-wide *reader*.

- **SSAR gate is the boundary.** The pre-flight checks `*/*` (the canonical admin check), so a
  non-admin operator is refused before anything is created — the escalation-prevention boundary
  (NFR-002, ADR-005) holds with no special handling.
- **Loud audit + warning.** A stderr warning records that the session grants all resources
  (admin-equivalent for the chosen verbs).
- **Rejections:** `*` mixed with other resources (`pods,*`); `*` with `--resource-name`; `*` with
  `--api-group` (the wildcard already spans all groups).
- **Acceptance:** an operator with the rights mints `get,list,watch` on `*` and the minted token
  can `get` `pods`, `configmaps`, and `services` but cannot `delete` `pods`; a limited operator who
  lacks `*/*` is refused with a non-zero exit and zero managed objects created.
- **Traces to:** ADR-014, ADR-006 · `scope_enforcement.feature` (@FR-019).

## Bet

_(For FR-019 — R-1 precommitted outcome.)_

- **Expected outcome**: `--resource '*' --verb get,list,watch -A` becomes the documented, recommended
  way to grant an agent ephemeral cluster-wide read, with **zero boundary regression** — the
  non-admin-refused `@FR-019` acceptance scenario stays GREEN.
- **Evidence method**: `@FR-019` scenarios GREEN in `docs/.sdlc/execution-log.jsonl`; README Usage
  documents the all-resources reader.
- **Owner**: Wagner Ignacio Pinto Junior
- **Review date**: 2026-09-23
- **Result**: _(pending — confirmed | rejected | inconclusive)_
- **Decision**: _(pending — continue | revise | revert)_
