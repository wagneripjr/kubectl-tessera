# Requirements — Non-Functional

Cross-cutting constraints. Several are security properties that are *demonstrated* by the
acceptance suite; one (NFR-002) is an architectural constraint enforced by code review and
the boundary gate, not by a black-box test.

## NFR-001: No token leakage

The minted token must never be written to `~/.kube/config`, passed as a process argument,
or echoed to shell history. The throwaway kubeconfig is mode `0600`.

- **Acceptance:** the throwaway file is `0600`; `~/.kube/config` is byte-identical before
  and after a mint; the token string does not appear in the process command line.
- **Traces to:** ADR-001 · `distribution_cli.feature`.

## NFR-002: Create as the invoking user — no impersonation, no privileged context

All RBAC objects are created using the invoking user's own credentials. The tool must never
switch to a privileged context or use impersonation to create objects. This is *the*
property that makes the non-admin case a real security boundary.

- **Acceptance (review-enforced):** no code path constructs a client from anything but the
  resolved `ConfigFlags` identity; `docs/BOUNDARIES.md` forbids an impersonation/privileged
  client. Verified by code review and the architecture-boundary gate, not a black-box test
  (a black-box test cannot prove the *absence* of a hidden privileged path).
- **Traces to:** ADR-005.

## NFR-003: Supply-chain trust — signed releases + SBOM

Because this tool mints credentials, releases must be cosign-keyless-signed and ship a
Software Bill of Materials.

- **Acceptance:** a release produces a cosign signature + certificate over `checksums.txt`
  and an SPDX-JSON SBOM per archive.
- **Traces to:** ADR-003 · release workflow.

## NFR-004: Cross-platform binaries

Ship binaries for `linux,darwin,windows × amd64,arm64`.

- **Acceptance:** `goreleaser build --snapshot` produces all six target binaries.
- **Traces to:** ADR-003.

## NFR-005: GC idempotency / cron-safety

`tessera gc` must be safe to run repeatedly and concurrently (host cron or in-cluster
CronJob).

- **Acceptance:** running gc twice in succession deletes the same expired set once and
  exits 0 both times; the second run makes no further deletions.
- **Traces to:** ADR-007 · `lifecycle_cleanup.feature` (#10).

## NFR-006: Fail-safe defaults

Defaults must never widen scope implicitly: verbs default to `get,list,watch`; scope is
namespaced unless `--cluster-scoped`; TTL defaults to `15m`.

- **Acceptance:** invoking `mint` with only `--resource pods` yields a `get,list,watch`,
  namespaced, 15m session.
- **Traces to:** ADR-004 · `scope_enforcement.feature`.

## NFR-007: License Apache-2.0

The project is licensed Apache-2.0 (Kubernetes-ecosystem norm + explicit patent grant).

- **Acceptance:** a top-level `LICENSE` contains the Apache-2.0 text; release archives
  include it.
- **Traces to:** ADR-002.

## NFR-008: stdout hygiene contract

In `--print-kubeconfig`, stdout must contain only the kubeconfig path (all diagnostics to
stderr) so `export KUBECONFIG=$(kubectl tessera … --print-kubeconfig)` works.

- **Acceptance:** stdout from a `--print-kubeconfig` mint is exactly one line, a filesystem
  path that exists and is a valid kubeconfig; the audit line and warnings appear only on
  stderr.
- **Traces to:** ADR-007 · `distribution_cli.feature` (#6).
