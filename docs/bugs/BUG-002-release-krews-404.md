# BUG-002: Release workflow always fails — goreleaser krew block 404s on empty index repo

**Status**: FIXED (2026-06-23)
**Severity**: Medium
**Found**: 2026-06-23

## Related

- **Requirement**: NFR-003 (signed releases + SBOM), NFR-004 (cross-platform binaries)
- **ADR**: ADR-003, ADR-012
- **Component**: `.goreleaser.yaml` `krews` block, `.github/workflows/release.yaml`

## Symptoms

Every tagged release run fails. Both `v0.1.0` (run 27979090214) and `v0.1.1`
(run 27981963396) ended in `failure` on the `GoReleaser` job, and the dependent
`krew` job (`needs: goreleaser`) was therefore **skipped — it has never run**, so the
krew-index automation path has never executed.

From the v0.1.1 job log:

```
• krew plugin manifest
  • writing                manifest=dist/krew/tessera.yaml
  • error checking for default branch   statusCode=404 error=GET https://api.github.com/repos//: 404 Not Found
⨯ release failed after 9m8s
  error= krew plugin manifest: could not get default branch: GET https://api.github.com/repos//: 404 Not Found
```

## Root Cause

goreleaser's `krews` pipe resolves the **target index repository's** default branch before
writing the manifest. With no `repository` (owner/name) configured, the request URL collapses
to `https://api.github.com/repos//` → 404 → the release exits 1. The inline comment claiming
`skip_upload: true` prevents this is **wrong**: `skip_upload` only skips the git *push* of the
rendered manifest; it does not skip the default-branch metadata lookup. The block was redundant
anyway — the krew-index PR is opened by `rajatjindal/krew-release-bot`, which consumes a
separate `.krew.yaml` template, not goreleaser's generated manifest.

Artifacts (archives, checksums, cosign bundle, SBOMs) are published *before* the krew step, which
is why v0.1.1's release assets exist and verify despite the job reporting failure (NFR-003/004
artifacts genuinely valid; only the workflow status was red).

## Impact

Release automation reliability: every release run is red and the krew-index update job never
runs, blocking the `kubectl krew install tessera` distribution milestone (VISION item 5). No
runtime or supply-chain impact — the published artifacts are correct and signed.

## Fix

Applied (2026-06-23):

1. Removed the `krews` block from `.goreleaser.yaml` (eliminates the 404 and the dual manifest
   source of truth).
2. Added `.krew.yaml` at the repo root as the single manifest template consumed by
   krew-release-bot (`addURIAndSha`, all 6 platforms).
3. Set goreleaser archive `name_template` to embed the full tag (`{{ .Tag }}`, e.g.
   `kubectl-tessera_v0.1.2_linux_amd64.tar.gz`) so the `.krew.yaml` `addURIAndSha` URL — whose
   inner render exposes only `{{ .TagName }}` and no string functions — matches the asset name
   with zero manipulation.
4. Gated the `krew` job off (`if: false`) until tessera is accepted into krew-index. (Initially
   `continue-on-error: true`, but the v0.1.2 run showed the bot *does* open a first PR — which
   krew maintainers auto-close, krew-index#5917 — so gating it off avoids filing an auto-closed
   PR on every tag.)

Verified: the `v0.1.2` Release run is green end-to-end (run 28069308953, `GoReleaser: success`)
— the first non-failing release in the project's history. The rendered manifest passed
krew-index's own `Validate plugin manifests` CI and a local `validate-krew-manifest` (all 6
platforms install fine) + `kubectl krew install --manifest --archive` smoke test.

## Test Gap

There is no acceptance/CI coverage of the release pipeline itself — it is exercised only by real
tags, and a red tag run was tolerated because the artifacts still uploaded. A `goreleaser check`
+ `goreleaser release --snapshot --clean` smoke step in CI would have caught a config that builds
locally but fails the publish-phase metadata lookup only against a real `release` target.

## Prevention

- Recommended: add a `goreleaser check` + `goreleaser release --snapshot --clean` step to the
  `CI` workflow guarded on `.goreleaser.yaml` changes, so a config that builds locally but fails
  the publish-phase metadata lookup is caught before tagging.
- Treat a red release run as release-blocking, never "assets uploaded, good enough".
