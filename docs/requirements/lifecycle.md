# Requirements — Lifecycle, GC & Discovery

Functional requirements for sweeping expired sessions, listing active sessions, discovery,
and multi-namespace handling.

## FR-011: `tessera gc` — sweep expired managed object sets

List objects carrying `app.kubernetes.io/managed-by=kubectl-tessera` across namespaces,
parse the `tessera.adustio.com/expires-at` annotation, and delete sets where
`now > expires-at`. Idempotent and cron-safe. Required for `--print-kubeconfig` (no
foreground process traps it) and for crash recovery.

- **Acceptance:** gc deletes only expired, managed object sets; it never touches unmanaged
  RBAC or unexpired sessions. Re-running is a no-op.
- **Traces to:** ADR-007 · NFR-005 · `lifecycle_cleanup.feature` (#5 crash recovery, #10 selectivity).

## FR-012: `tessera ls` — list active sessions

List active sessions (session-id, owner, scope summary, expires-at) derived from the
managed objects' labels/annotations.

- **Acceptance:** with zero managed objects, `tessera ls -o json` outputs `[]` (empty JSON
  array) and exits 0; the default table output is a header line plus zero data rows. With
  N active sessions, `-o json` outputs an array of N objects, each containing non-empty
  `sessionID`, `owner`, and `expiresAt` fields.
- **Traces to:** ADR-008 · unit tests + e2e.

## FR-013: SSRR discovery surfacing the `Incomplete` flag

Use `SelfSubjectRulesReview` for **discovery only** (`--dry-run`/`--explain`): show what the
operator could scope to. Surface `.Status.Incomplete` — for non-enumerable authorizers
(e.g. webhook) the rule list can be incomplete, and SSRR does not evaluate `resourceNames`.
SSRR is **never** the authorization gate (that is FR-003 / SSAR).

- **Acceptance:** when the SSRR response has `Status.Incomplete == true`, `--dry-run`
  writes a stderr line matching the regex `(?i)discovery may be incomplete` and still exits
  0. The notice-rendering path is unit-tested with a faked `Incomplete: true` SSRR
  response; the full webhook-authorizer scenario is manual (see ADR-011).
- **Traces to:** ADR-006, ADR-011 · `discovery.feature` (`@manual @webhook`) + preflight unit test.

## FR-016: Clear, actionable error handling for external preconditions

Surface precise errors for the common failure modes:

- Operator lacks `create` on `serviceaccounts`/`roles`/`rolebindings` → name the missing
  verbs and explain an admin must grant `create` (or `bind` on a curated role).
- Cluster is `< 1.24` (no TokenRequest API) → fail clearly. Target is 1.34.
- **Acceptance:**
  - Missing `create` on serviceaccounts exits code 1 and stderr contains the literal string
    `missing verb: create on serviceaccounts` (likewise `… on roles`, `… on rolebindings`
    for the respective kinds).
  - A cluster below 1.24 exits code 1 and stderr contains the literal string
    `requires Kubernetes >= 1.24 (TokenRequest API)`.
- **Traces to:** ADR-001 · unit tests + e2e.

## FR-017: Multi-namespace sessions

When an explicit list of namespaces is requested (`-n ns-a,ns-b`), grant the requested scope
in each of them as one atomically-managed session. To preserve the one-token model, a single
ServiceAccount (created in the first requested namespace) is the subject of one Role +
RoleBinding per requested namespace, so the single minted token reaches every listed
namespace and no others. All objects share one session-id.

- **Acceptance:** a two-namespace request yields one ServiceAccount plus a Role+RoleBinding
  per requested namespace, all sharing one session-id; the minted credential is allowed in
  each requested namespace and denied in a namespace that was not requested; `gc`/cleanup
  removes the whole set as a unit.
- **Traces to:** ADR-008 · e2e (`multi_namespace.feature`).

## FR-018: All-namespaces sessions (wildcard)

When all namespaces are requested (`-A`/`--all-namespaces`, with `-n '*'` accepted as sugar),
grant the requested scope over the namespaced resources in EVERY namespace — including
namespaces created after the mint — via a single ClusterRole + ClusterRoleBinding bound to
one ServiceAccount. This is the widest scope tessera can mint, so it is never a default
(NFR-006), is gated cluster-wide (the SSAR pre-flight checks each rule with an empty
namespace, so an operator who cannot already exercise the scope in every namespace is
refused before anything is created), and emits a loud cluster-wide audit warning. `-A` cannot
be combined with `--cluster-scoped` (which targets cluster-scoped resource *types*) nor with
an explicit namespace list; a cluster-scoped resource type under `-A` is rejected with a
pointer to `--cluster-scoped`.

- **Acceptance:** an all-namespaces read session is allowed to read the resource in the
  current namespace AND in a namespace created after the mint, and is denied the unrequested
  verb; an operator who may only read in one namespace is refused, and the refusal creates no
  managed objects anywhere (no leaked ClusterRoleBinding).
- **Traces to:** ADR-013 · e2e (`multi_namespace.feature`).
