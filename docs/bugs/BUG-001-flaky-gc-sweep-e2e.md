# BUG-001: Flaky E2E — gc sweep sometimes leaves an expired session

**Status**: FIXED (2026-06-23)
**Severity**: Low
**Found**: 2026-06-21

## Related

- **Requirement**: FR-011 (gc sweep of expired managed RBAC sets), NFR-005
- **ADR**: —
- **Component**: `internal/gc` sweep + acceptance scenario
  `the sweep removes only expired managed sessions`

## Symptoms

The acceptance scenario `the_sweep_removes_only_expired_managed_sessions` intermittently
fails on the `kindest/node:v1.34.8` E2E matrix leg with:

```
expected managed session "expired1" to be removed
```

It is non-deterministic: the same commit passes on a re-run and on the
`v1.34.8` leg vs `v1.36.1` leg inconsistently. Observed across the latest master run
(green), an earlier master run (red), and PR #9 (red then green on retry) — code in
`internal/gc` unchanged across all three, which rules out a code regression.

## Root Cause

Confirmed: the assertion, not the sweep. `gc` deletes the expired set with **foreground
propagation** (it returns before the API server finishes removing the objects). The DSL step
`ThenManagedSessionRemoved` (`test/dsl/session_dsl.go`) then read `SessionObjectsExist` **exactly
once, synchronously**. On a slower/older kind node the single read races the still-propagating
deletion and intermittently still sees `expired1`. Its sibling `ThenSessionObjectsGone` already
polled against a deadline for the same reason — `ThenManagedSessionRemoved` simply never got the
same treatment. `internal/gc` was correct the whole time (consistent with the symptom being
node-version- and timing-dependent, never a code regression).

## Impact

CI reliability only — produces false-red runs on PRs and master, requiring a re-run. No
production/runtime impact: the real `tessera gc` is idempotent and reclaims orphans on the
next sweep.

## Fix

Applied (2026-06-23): `ThenManagedSessionRemoved` now polls `SessionObjectsExist` against a 5s
deadline (200ms interval), returning as soon as the objects are gone and only failing if they
persist past the deadline — identical to the existing `ThenSessionObjectsGone`. Test-infrastructure
only; no production/`internal/gc` change. The seeded `expires-at` is already comfortably in the past
(`time.Now().Add(-time.Hour)`), so the deletion-propagation poll is the complete fix.

## Test Gap

The scenario asserts post-sweep state once, synchronously, with no tolerance for
create/list/delete propagation latency on a freshly-provisioned cluster.

## Prevention

Use eventually-style assertions for cluster-state effects in acceptance tests; never assert
asynchronous Kubernetes reconciliation/visibility with a single immediate read.
