# kubectl-tessera — Project Instructions

`kubectl-tessera` is an open-source kubectl plugin (invoked as `kubectl tessera`) that
mints ephemeral, scope-narrowed, TTL-bound Kubernetes credentials, running as the invoking
user, with an SSAR pre-flight gate and automatic RBAC cleanup. Authoritative design:
`docs/plans/kubectl-tessera-implementation-plan.md`. Requirements: `docs/requirements/`.
Decisions: `docs/adr/`. This file captures the invariants that are easy to get wrong.

## Locked parameters (change in one coordinated pass or not at all)

- **Module:** `github.com/wagneripjr/kubectl-tessera` · **binary:** `kubectl-tessera` ·
  **krew name:** `tessera` (no `kubectl-` prefix in the krew manifest `name`).
- **Label domain:** `tessera.adustio.com`. Keys: `tessera.adustio.com/owner`,
  `/session-id`; annotation `/expires-at`. Plus `app.kubernetes.io/managed-by=kubectl-tessera`.
  Defined once in `internal/labels`; the acceptance suite keeps its own copy on purpose
  (black-box contract — ADR-008).
- **License:** Apache-2.0. **Default branch:** `master`.
- **Version:** from git tags via ldflags (`main.version/commit/date`); no VERSION file.
  `kubectl tessera --version` and `tessera version` must work.

## Hard "do NOT" list (these void the security design)

1. **Do NOT** create objects with a privileged context or via impersonation — create as the
   invoking user. This is the only thing that makes the non-admin case a real boundary
   (NFR-002, ADR-005). Enforced by `docs/BOUNDARIES.md`.
2. **Do NOT** use `SelfSubjectRulesReview` as the authorization gate. Gate with
   `SelfSubjectAccessReview`; SSRR is discovery only and must surface its `Incomplete` flag
   (ADR-006).
3. **Do NOT** leave orphaned objects on partial failure — reverse-order rollback (FR-005).
4. **Do NOT** default to wide scope — `get,list,watch`, namespaced, 15m (NFR-006).
5. **Do NOT** write the token to `~/.kube/config` or pass it on the command line; kubeconfig
   is `0600` (NFR-001).

## Flag naming

`ConfigFlags` (k8s.io/cli-runtime) owns the global `--cluster` (kubeconfig cluster name).
tessera's "cluster-scoped resources" flag is therefore **`--cluster-scoped`**, NOT `--cluster`.
Do not reintroduce the collision (ADR-001).

## Testing — ATDD with godog

Features follow the double-loop ATDD cycle (invoke `wagner-skills:atdd`). Acceptance specs are
Gherkin in `specs/features/` (QA-owned, business language only). The protocol driver
(`test/drivers`, build tag `e2e`) is a composite: process adapter (spawns the binary) +
cluster adapter (client-go, verifies the MINTED token's real authz via `auth can-i` and real
requests). Assertions live ONLY in `test/dsl` (Gate G5). The acceptance suite is black-box —
it must NOT import `internal/`.

- Fast unit loop: `go test ./...` (no cluster).
- Acceptance: `go test -tags=e2e ./test/e2e/...` against a kind cluster; select with
  `GODOG_TAGS` (default excludes `@manual`). See ADR-009, ADR-010.
- The cluster-admin caveat (VISION.md) is load-bearing: never describe the tool as containing
  a malicious or mistaken cluster-admin. It is an accident-limiter for the agent and a real
  boundary only for non-admins.

## Conventions

- Conventional commits referencing `(FR|NFR|BUG)-NNN` for `feat/fix/perf` (ID-exempt for
  `docs/chore/test/ci/refactor/build/style`). Update `docs/TRACEABILITY.md` when requirements,
  bugs, or ADRs change.
- New bugs: `docs/bugs/BUG-NNN-slug.md` + a TRACEABILITY row.
