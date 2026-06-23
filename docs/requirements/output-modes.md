# Requirements — Output Modes & Kubeconfig

Functional requirements for how a minted session is delivered to the caller and observed.

## FR-007: Build an isolated 0600 throwaway kubeconfig

Build a `clientcmdapi.Config` reusing the source context's cluster `server` + CA (and any
proxy URL), with an AuthInfo holding **only** the minted token and a Context bound to the
target namespace. Write it `0600` to an isolated path
(`${XDG_RUNTIME_DIR:-/tmp}/tessera/<sessionID>.kubeconfig`).

- **Acceptance:** the file is mode `0600`; it contains the token but never the operator's
  other credentials; `~/.kube/config` is untouched; the token never appears in argv.
- **Traces to:** ADR-001 · NFR-001 · `distribution_cli.feature` (#6).

## FR-008: `--print-kubeconfig` output mode

Print **only** the kubeconfig path to stdout (all logs to stderr) and exit 0, leaving the
managed objects in place for `gc` to reclaim. This is the mode for non-interactive callers
(AI agents).

- **Acceptance:** `export KUBECONFIG=$(kubectl tessera … --print-kubeconfig)` yields a
  usable scoped kubeconfig; stdout contains nothing but the path.
- **Traces to:** ADR-007 · NFR-008 · `distribution_cli.feature` (#6).

## FR-009: `--exec` subshell mode (default)

Spawn `${SHELL:-/bin/bash}` with `KUBECONFIG` set to the throwaway file; on subshell exit
or `SIGINT`/`SIGTERM`, delete the session's object set and remove the kubeconfig file.
`--exec` and `--print-kubeconfig` are mutually exclusive; `--exec` is the default.

- **Acceptance:** exiting the subshell removes the SA/Role/RoleBinding and the kubeconfig
  file. The mechanism must be drivable without a TTY (for testing) and expose the child
  process so it can be force-killed (crash-recovery test).
- **Traces to:** ADR-007 · `lifecycle_cleanup.feature` (#4, #5).

## FR-010: `--dry-run`

Run the pre-flight gate and print the intended objects; create nothing. Surfaces SSRR
discovery output including the `Incomplete` flag (see FR-013).

- **Acceptance:** no managed objects are created; the intended object set is printed.
- **Traces to:** ADR-006 · `discovery.feature`.

## FR-014: Audit line to stderr on every mint

On every successful mint, emit a single audit line to **stderr**: session-id, owner,
scope, requested TTL, effective expiry, namespace(s), cluster.

- **Acceptance:** stderr contains the audit line; stdout hygiene (FR-008) is preserved.
- **Traces to:** ADR-008 · `distribution_cli.feature`.

## FR-015: `-o json` machine-readable output

Provide `-o json` emitting a machine-readable session descriptor (session-id, scope,
effective expiry, kubeconfig path, created objects).

- **Acceptance:** `-o json` output parses as JSON and contains the session fields.
- **Traces to:** ADR-008 · unit tests + `distribution_cli.feature`.

## FR-020: Help on bare invocation + usage examples

Invoked with no arguments and no flags, `kubectl tessera` prints its help (usage, flags, and
worked examples) to **stdout** and exits **0** — a discovery gesture, not a failed mint. This
is the friendly first contact for new users and AI agents, who would otherwise hit
`--resource is required`. Any flag or positional argument preserves the existing behavior:
`kubectl tessera --resource pods` mints; `kubectl tessera -n prod` still fails the
`--resource is required` pre-check; a stray positional argument is not swallowed into help.
The root command and every subcommand (`version`, `gc`, `ls`) carry an `Example:` block of
common, copy-pasteable use cases, and help renders the binary as `kubectl tessera` (via the
Cobra display-name annotation), not `kubectl-tessera`.

- **Acceptance:** `kubectl tessera` with no args exits 0 and its stdout contains a usage line,
  an `Examples:` section, and the `--resource` flag; `kubectl tessera -n prod` still exits
  non-zero with `--resource is required`; a stray positional argument yields a non-help error.
- **Traces to:** ADR-001 (Cobra) · unit tests (`internal/cli/help_test.go`).

## Bet

_(For FR-020 — R-1 precommitted outcome.)_

- **Expected outcome**: a first-time `kubectl tessera` (no args) returns actionable help with
  ≥ 5 worked examples and exit 0 instead of an error, lowering time-to-first-success for new
  users and AI agents — with **zero mint-path regression** (the `--resource is required`
  assertion stays GREEN).
- **Evidence method**: `internal/cli/help_test.go` GREEN (exit 0 + `Examples:` present + mint
  path preserved) in `docs/.sdlc/execution-log.jsonl`; README Usage and the Pages site show the
  same common-use-case examples.
- **Owner**: Wagner Ignacio Pinto Junior
- **Review date**: 2026-09-23
- **Result**: _(pending — confirmed | rejected | inconclusive)_
- **Decision**: _(pending — continue | revise | revert)_
