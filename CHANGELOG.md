# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-06-22

First public release.

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

[Unreleased]: https://github.com/wagneripjr/kubectl-tessera/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/wagneripjr/kubectl-tessera/releases/tag/v0.1.0
