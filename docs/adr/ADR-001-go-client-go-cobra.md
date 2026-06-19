---
title: "ADR-001: Go + client-go + Cobra + cli-runtime, no shelling to kubectl"
status: Accepted
date: 2026-06-19
authors:
  - Wagner Ignacio Pinto Junior
tags:
  - language
  - kubernetes
  - cli
supersedes: null
superseded_by: null
---

# ADR-001: Go + client-go + Cobra + cli-runtime, no shelling to kubectl

## Status

**Accepted**

## Context

`kubectl-tessera` is a kubectl plugin that performs authorization reviews (SSAR/SSRR),
creates RBAC objects, mints TokenRequest tokens, and writes kubeconfigs. It must resolve
cluster configuration *exactly* the way kubectl does (kubeconfig, context, namespace,
in-cluster, exec credential plugins) and must never depend on a `kubectl` binary being on
PATH or on its output format.

## Decision

Implement in **Go** using **`k8s.io/client-go`** (typed clientsets, discovery, RESTMapper,
TokenRequest), **`spf13/cobra`** for commands, and **`k8s.io/cli-runtime`
`genericclioptions.ConfigFlags`** for standard config flags. Never shell out to `kubectl`.

`ConfigFlags` provides `--kubeconfig`, `--context`, `--namespace/-n`, `--cluster`,
`--user`, `--server`, etc., and resolves config identically to kubectl. Because
`ConfigFlags` owns `--cluster` (the kubeconfig cluster name), tessera's "cluster-scoped
resources" flag is named **`--cluster-scoped`** (see `docs/requirements/minting.md` FR-002).

## Consequences

### Positive

- **POS-001**: Config resolution is identical to kubectl — no surprises for users.
- **POS-002**: Direct API access gives typed errors and the real TokenRequest/SSAR APIs.
- **POS-003**: Single static binary; trivial krew distribution.

### Negative

- **NEG-001**: client-go version must track the target server (1.34) and is a heavy dep.
- **NEG-002**: `--cluster` flag-name collision forced the `--cluster-scoped` rename.

## Alternatives Considered

### Alternative 1: Shell out to `kubectl auth can-i` / `kubectl create`

**Rejected because**: brittle output parsing, requires kubectl on PATH, loses typed errors
and the returned TokenRequest `ExpirationTimestamp`, and complicates "create as the
invoking user".

### Alternative 2: Another language (Rust/Python) with a k8s client

**Rejected because**: client-go is the reference implementation; krew/goreleaser tooling and
the ecosystem assume Go; static binaries are simplest in Go.

## References

- ADR-005 (create-as-user), ADR-006 (SSAR/SSRR), ADR-008 (label schema)
- `k8s.io/cli-runtime/pkg/genericclioptions`
