# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.2] - 2026-06-23

First release whose pipeline completes green. v0.1.0 and v0.1.1 both reported failure —
their artifacts published, but the run exited 1 — so the krew-index automation never ran.

### Fixed

- Release workflow no longer fails on every tag: removed the goreleaser `krews` block, which
  404'd resolving an unconfigured index repo's default branch even under `skip_upload`
  (BUG-002). The krew-index manifest is now owned solely by `.krew.yaml` + `krew-release-bot`.

### Changed

- Release archive filenames embed the full tag (`kubectl-tessera_v0.1.2_<os>_<arch>`) so the
  krew manifest template resolves asset URLs from `{{ .TagName }}` with no version-string
  manipulation.

## [0.1.1] - 2026-06-22

First public release. (v0.1.0 was tagged but never published — its release pipeline
failed at cosign artifact signing; v0.1.1 ships the fix.)

### Added

- Mint ephemeral, scope-narrowed, TTL-bound Kubernetes credentials as the invoking
  user, with a `SelfSubjectAccessReview` pre-flight gate and reverse-order RBAC rollback
  on partial failure (FR-001–FR-006).
- Scope resolution via RESTMapper; `--cluster-scoped` for cluster-scoped resources;
  fail-safe defaults (`get,list,watch`, namespaced, 15m TTL) (FR-002, NFR-006).
- Output modes: isolated `0600` throwaway kubeconfig, `--print-kubeconfig`, `--exec`
  subshell with signal-trap cleanup, `--dry-run`, and `-o json` (FR-007–FR-010, FR-015).
- `tessera gc` expired-session sweep (idempotent, cron-safe) and `tessera ls`
  active-session inventory (FR-011, FR-012, NFR-005).
- Multi-namespace sessions (one ServiceAccount, a Role+RoleBinding per namespace) and the
  `-A/--all-namespaces` wildcard (ClusterRole+ClusterRoleBinding) (FR-017, FR-018).
- stderr audit line and clear precondition errors (FR-014, FR-016).
- Signed (keyless cosign) and SBOM'd (syft SPDX) cross-platform release via goreleaser,
  plus a krew plugin manifest (NFR-001, NFR-003, NFR-004).

[Unreleased]: https://github.com/wagneripjr/kubectl-tessera/compare/v0.1.2...HEAD
[0.1.2]: https://github.com/wagneripjr/kubectl-tessera/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/wagneripjr/kubectl-tessera/releases/tag/v0.1.1
