# Traceability Matrix — kubectl-tessera

Bidirectional map: requirements ↔ ADRs ↔ acceptance specs ↔ tests. Spec coverage is
`X/Y GREEN` once tests pass, or `0/Y TODO` while uncovered (scaffolding stage — all
behavior is unimplemented by design; see the project plan).

## Requirements → Implementation

| Req ID | Description | ADR(s) | Feature File | Spec Coverage | Status |
|--------|-------------|--------|--------------|---------------|--------|
| FR-001 | Mint ephemeral scoped credential | ADR-001, ADR-004 | scope_enforcement.feature | 3/3 GREEN | Done |
| FR-002 | Scope resolution via RESTMapper (`--cluster-scoped` rename) | ADR-001 | scope_enforcement.feature + unit | 2/2 GREEN | Done |
| FR-003 | SSAR pre-flight gate | ADR-006 | preflight_gate.feature | 1/1 GREEN | Done |
| FR-004 | Create managed RBAC set as invoking user | ADR-005, ADR-008 | scope_enforcement.feature | 1/1 GREEN | Done |
| FR-005 | Reverse-order rollback, no orphans | ADR-005 | lifecycle_cleanup.feature | unit GREEN; e2e fault-injection still a placeholder | In Progress |
| FR-006 | TokenRequest mint + clamp warning | ADR-001 | lifecycle_cleanup.feature | unit GREEN; e2e expiry blocked by cluster min token TTL | In Progress |
| FR-007 | Isolated 0600 throwaway kubeconfig | ADR-001 | distribution_cli.feature | 1/1 GREEN | Done |
| FR-008 | `--print-kubeconfig` output mode | ADR-007 | distribution_cli.feature | 1/1 GREEN | Done |
| FR-009 | `--exec` subshell + signal-trap cleanup | ADR-007 | lifecycle_cleanup.feature | 0/2 TODO | Pending |
| FR-010 | `--dry-run` | ADR-006 | discovery.feature + unit | 0/1 TODO | Pending |
| FR-011 | `tessera gc` expired sweep | ADR-007 | lifecycle_cleanup.feature | 0/2 TODO | Pending |
| FR-012 | `tessera ls` active sessions | ADR-008 | (unit + e2e) | 0/1 TODO | Pending |
| FR-013 | SSRR discovery + `Incomplete` notice | ADR-006, ADR-011 | discovery.feature (@manual) + preflight unit | 0/1 TODO | Pending |
| FR-014 | stderr audit line | ADR-008 | distribution_cli.feature | 1/1 GREEN | Done |
| FR-015 | `-o json` output | ADR-008 | distribution_cli.feature + unit | 0/1 TODO | Pending |
| FR-016 | Clear errors (lacks-create; k8s<1.24) | ADR-001 | unit + e2e | 0/2 TODO | Pending |
| FR-017 | Multi-namespace one-set-per-ns | ADR-008 | (e2e) | 0/1 TODO | Pending |
| NFR-001 | No token leakage + 0600 | ADR-001 | distribution_cli.feature | 1/1 GREEN | Done |
| NFR-002 | Create-as-user, no impersonation | ADR-005 | BOUNDARIES + code review | review-only by design | Pending |
| NFR-003 | Signed releases + SBOM | ADR-003 | release workflow | CI-verified | Pending |
| NFR-004 | Cross-platform binaries | ADR-003 | goreleaser snapshot | CI-verified | Pending |
| NFR-005 | gc idempotency / cron-safe | ADR-007 | lifecycle_cleanup.feature | 0/1 TODO | Pending |
| NFR-006 | Fail-safe defaults | ADR-004 | scope_enforcement.feature | 1/1 GREEN | Done |
| NFR-007 | Apache-2.0 license | ADR-002 | LICENSE present | n/a | Pending |
| NFR-008 | stdout hygiene contract | ADR-007 | distribution_cli.feature | 1/1 GREEN | Done |

> Acceptance criteria #1–#11 (from the implementation plan) map onto the feature files above;
> the per-criterion grouping is in `specs/features/` and the project plan. Criterion #11 (SSRR
> `Incomplete`) is the only one not fully automatable in standard CI — covered by a unit
> surrogate + a `@manual @webhook` e2e scenario (ADR-011).

## Bugs → Traceability

| Bug ID | Severity | Status | Related Req | Related ADR | Fix Commit |
|--------|----------|--------|-------------|-------------|------------|
| — | — | — | — | — | — |

_No bugs yet. New bugs go in `docs/bugs/BUG-NNN-slug.md` and add a row here._

## ADRs → Requirements

| ADR | Title | Status | Requirements |
|-----|-------|--------|--------------|
| ADR-001 | Go + client-go + Cobra + cli-runtime | Accepted | FR-001, FR-002, FR-006, FR-007, FR-016, NFR-001 |
| ADR-002 | Apache-2.0 license | Accepted | NFR-007 |
| ADR-003 | krew + goreleaser distribution | Accepted | NFR-003, NFR-004 |
| ADR-004 | Fail-safe defaults | Accepted | FR-001, NFR-006 |
| ADR-005 | Create as invoking user | Accepted | FR-004, FR-005, NFR-002 |
| ADR-006 | SSAR gate / SSRR discovery | Accepted | FR-003, FR-010, FR-013 |
| ADR-007 | Three-layer cleanup | Accepted | FR-008, FR-009, FR-011, NFR-005, NFR-008 |
| ADR-008 | Label/annotation schema | Accepted | FR-004, FR-012, FR-014, FR-015, FR-017 |
| ADR-009 | ATDD via godog on kind | Accepted | (testing) all FRs via acceptance |
| ADR-010 | e2e gating (build tag + GODOG_TAGS) | Accepted | (testing infrastructure) |
| ADR-011 | SSRR Incomplete coverage | Accepted | FR-013 |
| ADR-012 | krew-release-bot for krew-index PR | Accepted | NFR-003 |
