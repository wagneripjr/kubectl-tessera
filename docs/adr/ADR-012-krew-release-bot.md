---
title: "ADR-012: Open the krew-index PR via krew-release-bot"
status: Accepted
date: 2026-06-19
authors:
  - Wagner Ignacio Pinto Junior
tags:
  - distribution
  - ci
supersedes: null
superseded_by: null
---

# ADR-012: Open the krew-index PR via krew-release-bot

## Status

**Accepted**

## Context

goreleaser generates a valid krew manifest, but opening a pull request to
`kubernetes-sigs/krew-index` (a repository we don't own) from the release workflow requires
GoReleaser **Pro**. We want the official-index PR automated without a paid tier.

## Decision

Let goreleaser **generate and sign** the manifest, and use the free, maintainer-recognized
**`rajatjindal/krew-release-bot`** GitHub Action to open the PR to `kubernetes-sigs/krew-index`
on tag. Validate the manifest with krew's `validate-krew-manifest` before release to avoid PR
rejections.

## Consequences

### Positive

- **POS-001**: Fully automated krew-index PR with no paid tooling.
- **POS-002**: Decouples manifest generation (goreleaser) from cross-repo PR mechanics.

### Negative

- **NEG-001**: Depends on a third-party action; pin it and review updates.
- **NEG-002**: Until krew-index acceptance, install is via `go install` or a custom index
  (documented in README).

## Alternatives Considered

### Alternative 1: GoReleaser Pro `krews.repository` cross-repo push

**Rejected because**: paid; no added value over the free bot for this project.

### Alternative 2: Manually open the krew-index PR each release

**Rejected because**: error-prone and easy to forget; automation is cheap here.

## References

- ADR-003 · NFR-003 · .github/workflows/release.yaml
