---
title: "ADR-003: Distribute via krew + goreleaser with signed, SBOM'd releases"
status: Accepted
date: 2026-06-19
authors:
  - Wagner Ignacio Pinto Junior
tags:
  - distribution
  - supply-chain
supersedes: null
superseded_by: null
---

# ADR-003: Distribute via krew + goreleaser with signed, SBOM'd releases

## Status

**Accepted**

## Context

Users expect `kubectl krew install tessera`. krew requires versioned, hosted, cross-platform
archives with sha256 sums. As a credential-minting tool, the release path needs a high
supply-chain trust bar.

## Decision

Use **goreleaser** to build the `linux,darwin,windows × amd64,arm64` matrix, produce
`.tar.gz`/`.zip` archives + `checksums.txt`, generate the **krew** manifest, **cosign-keyless**
sign the checksums, and emit **syft** SPDX-JSON SBOMs. Publish to GitHub Releases on tag.

## Consequences

### Positive

- **POS-001**: One config produces the full cross-platform, signed, SBOM'd release.
- **POS-002**: Meets krew-index hosting/checksum requirements out of the box.
- **POS-003**: Keyless signing needs no long-lived key (OIDC at release time).

### Negative

- **NEG-001**: Release workflow needs `id-token: write` and network access to Sigstore.
- **NEG-002**: Opening the official krew-index PR from a repo we don't own needs extra
  tooling (see ADR-012).

## Alternatives Considered

### Alternative 1: Hand-rolled `make release` + manual upload

**Rejected because**: error-prone, no signing/SBOM, no krew manifest generation.

### Alternative 2: GoReleaser Pro for cross-repo krew PR

**Rejected because**: paid; the free `krew-release-bot` covers the krew-index PR (ADR-012).

## References

- NFR-003, NFR-004 · ADR-012 · goreleaser v2 docs
