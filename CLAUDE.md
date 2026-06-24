# kubectl-tessera — Project Instructions

`kubectl-tessera` is an open-source kubectl plugin (invoked as `kubectl tessera`) that
mints ephemeral, scope-narrowed, TTL-bound Kubernetes credentials, running as the invoking
user, with an SSAR pre-flight gate and automatic RBAC cleanup. Authoritative design:
`docs/plans/kubectl-tessera-implementation-plan.md`. Requirements: `docs/requirements/`.
Decisions: `docs/adr/`. This file captures the invariants that are easy to get wrong.

## Build, lint, test

```bash
go build ./...                       # compile everything (CI gate)
go install ./cmd/kubectl-tessera     # put kubectl-tessera on PATH → `kubectl tessera` works
                                     # (version shows the dev placeholder; real version comes from goreleaser ldflags)
golangci-lint run                    # lint (v2 config; CI pins golangci-lint v2.12.2)
golangci-lint fmt                    # apply gofumpt + goimports (local-prefix github.com/wagneripjr/kubectl-tessera)

go test ./... -race -count=1         # full unit loop, exactly as CI runs it (no cluster)
go test ./internal/scope/ -run TestResolve   # one package / one test
```

E2E (needs a real cluster — CI matrixes kind v1.34 and v1.36):

```bash
kind create cluster                                      # once; suite runs against the current kube-context
GODOG_TAGS="@e2e && ~@manual" go test -tags=e2e ./test/e2e/... -v   # default selection
GODOG_TAGS="@FR-001" go test -tags=e2e ./test/e2e/... -v           # scenarios for one requirement
```

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

## Architecture — the mint pipeline

`cmd/kubectl-tessera/main.go` only injects build vars and calls `internal/cli`. Every command
is a thin Cobra shell over one single-purpose `internal/` package; the orchestration lives in
`internal/cli/run.go`. The default `mint` flow (read it top-to-bottom — the order *is* the security
design) is:

1. `validate()` flag combinations → resolve namespace scope (`-A`/`*`/list/`--cluster-scoped`).
2. `configFlags.ToRESTConfig()` → clientset **as the invoking user** (no impersonation — ADR-005).
3. `token.RequireSupported` (TokenRequest API present) → `scope.Resolve` (verbs+resources →
   RBAC `PolicyRule`s + resolved GVRs via the RESTMapper).
4. `preflight.Check` runs **two** SelfSubjectAccessReview gates: can you exercise the scope you
   asked for, *and* can you create the RBAC objects? Either denial aborts before any write (ADR-006).
5. `rbac.Create` → SA + (Cluster)Role + (Cluster)RoleBinding, stamped with `internal/labels`.
   Any later failure triggers reverse-order `rbac.Rollback` (FR-005).
6. `token.Mint` → bound TokenRequest for the SA (TTL may be floored/clamped by the cluster).
7. `kubeconfig.Build`/`Write` → `0600` file in its own dir, never `~/.kube/config` (NFR-001).
8. Terminal mode: `subshell.Run` spawns `$SHELL` with `KUBECONFIG` set and a `Cleanup` that rolls
   back RBAC + removes the kubeconfig on exit. `--print-kubeconfig` skips the subshell and leaves
   objects for `gc`; `--dry-run` stops after the gates and prints intended objects.

Subcommands: `gc` (`internal/gc`) sweeps expired/orphaned managed objects by label+`/expires-at`
annotation, cron-safe (`deploy/gc-cronjob.yaml`); `ls` (`internal/session`) lists active sessions by
label. Support packages: `internal/output` (json/table), `internal/session` (`Descriptor` shape),
`internal/labels` (the only place the label/annotation schema is defined).

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
