# Glossary — kubectl-tessera

Shared vocabulary. Terms here are used consistently across requirements, ADRs, specs, and
code.

- **Session** — one invocation of `mint`. Produces a set of managed RBAC objects (possibly
  across namespaces), a minted token, and a throwaway kubeconfig, all sharing one session-id.
- **Session-id** — the first 8 lowercase chars of a UUID, stamped as
  `tessera.adustio.com/session-id` on every object of a session. Makes a session addressable
  atomically.
- **Owner** — the sanitized identity of the invoking user (DNS-1123 safe), stamped as
  `tessera.adustio.com/owner`.
- **Scope** — the requested `(verbs × resources × namespaces[ × resourceNames][, apiGroup])`
  that the minted credential is confined to.
- **Managed object** — any object tessera created, identified by
  `app.kubernetes.io/managed-by=kubectl-tessera`. The safety selector for gc/ls/cleanup.
- **TTL** — requested lifetime (Go duration; default `15m`). The *effective* expiry is the
  API server's returned `ExpirationTimestamp`, which may be clamped shorter.
- **expires-at** — RFC3339 UTC annotation (`tessera.adustio.com/expires-at`) driving gc.
- **SSAR** — `SelfSubjectAccessReview`. Authoritative yes/no per rule. The pre-flight **gate**.
- **SSRR** — `SelfSubjectRulesReview`. Enumerates possible rules for **discovery only**; may be
  `Incomplete`; ignores `resourceNames`. Never the gate.
- **RESTMapper** — discovery-backed mapping from a resource name to its GVR/GVK and
  namespaced-vs-cluster scope.
- **Throwaway kubeconfig** — an isolated `0600` kubeconfig holding only the minted token, the
  source cluster's server+CA, and a context bound to the target namespace. Never the user's
  `~/.kube/config`.
- **Over-ask** — a requested scope exceeding what the operator is authorized for; rejected by
  the SSAR gate before any object is created.
- **Cluster-scoped** (`--cluster-scoped`) — a scope over cluster-scoped resources (e.g. nodes),
  producing a `ClusterRole` + `ClusterRoleBinding`. Distinct from kubectl's `--cluster` (the
  kubeconfig cluster name, owned by `ConfigFlags`).
- **Protocol driver** — the ATDD adapter that drives the system through its real external
  protocols: a process adapter (the CLI) + a cluster adapter (client-go). See
  `docs/design/protocol-drivers.md`.
- **Walking skeleton** — a minimal compiling build that wires the CLI and packages but
  implements no feature behavior; the honest RED target for the first acceptance run.
- **Limited identity** — a deliberately under-privileged ServiceAccount the acceptance suite
  runs tessera *as*, to exercise the non-admin security boundary (over-ask refusal, name
  denial).
