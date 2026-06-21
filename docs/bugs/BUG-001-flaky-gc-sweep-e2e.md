# BUG-001: Flaky E2E — gc sweep sometimes leaves an expired session

**Status**: OPEN
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

Not yet confirmed. Hypothesis: a timing/boundary race in the sweep's expiry decision —
the scenario seeds `expired1` with a past `tessera.adustio.com/expires-at`, invokes the
sweep, and asserts deletion. On a slower kind node (or near the `time.Now()` boundary, or
with slower API-server propagation of the create before the sweep lists), the object is not
yet visible/deleted when the assertion runs.

## Impact

CI reliability only — produces false-red runs on PRs and master, requiring a re-run. No
production/runtime impact: the real `tessera gc` is idempotent and reclaims orphans on the
next sweep.

## Fix

Not yet applied. Likely directions (to confirm against a local kind cluster):
- Assert deletion with a bounded poll/eventually rather than a single immediate check.
- Ensure the seeded object is observed (read-after-write) before invoking the sweep.
- Seed `expires-at` comfortably in the past (not near `now`).

## Test Gap

The scenario asserts post-sweep state once, synchronously, with no tolerance for
create/list/delete propagation latency on a freshly-provisioned cluster.

## Prevention

Use eventually-style assertions for cluster-state effects in acceptance tests; never assert
asynchronous Kubernetes reconciliation/visibility with a single immediate read.
