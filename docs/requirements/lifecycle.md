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

When multiple namespaces are requested, create one managed object set per namespace,
sharing the session-id so the session is operated on atomically.

- **Acceptance:** a two-namespace request yields one SA+Role+RoleBinding set per namespace,
  all sharing one session-id; `gc`/cleanup removes them as a unit.
- **Traces to:** ADR-008 · e2e.
