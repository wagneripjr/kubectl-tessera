# kubectl-tessera

Mint ephemeral, scope-narrowed, TTL-bound Kubernetes credentials that run as *you*, with an
SSAR pre-flight gate and automatic RBAC cleanup. Emits a throwaway, time-boxed kubeconfig for
least-privilege sessions — for example, pointing an AI agent at a cluster read-only for one hour.

[![Go Report Card](https://goreportcard.com/badge/github.com/wagneripjr/kubectl-tessera)](https://goreportcard.com/report/github.com/wagneripjr/kubectl-tessera)
[![CI](https://github.com/wagneripjr/kubectl-tessera/actions/workflows/ci.yaml/badge.svg)](https://github.com/wagneripjr/kubectl-tessera/actions/workflows/ci.yaml)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://github.com/wagneripjr/kubectl-tessera/blob/master/LICENSE)

**Website & downloads:** https://wagneripjr.github.io/kubectl-tessera/ — install instructions, usage, and the latest signed release for your platform.

## ⚠️ Read this first: what this tool actually buys you

The minted token is **always narrow** — whoever uses it is confined to exactly the scope you
requested. That part is unconditional. What is *conditional* is the guarantee about **your own**
privilege as the operator who runs `kubectl tessera`:

- **If you are NOT cluster-admin:** Kubernetes RBAC escalation-prevention makes this a **real
  security boundary**. You physically cannot mint a token with more than you already hold — the API
  server rejects binding to a Role whose rules you don't possess, unless you hold the `bind` verb on
  that Role. Tessera leans on this; it doesn't reinvent it.

- **If you ARE cluster-admin:** escalation-prevention is vacuous (you possess everything), so the
  tool **cannot stop you from over-scoping**. It remains an effective, self-imposed blast-radius
  guardrail for whatever you hand the token to (an agent, a script, a teammate), but it is **NOT a
  containment control on a privileged human**. The hard backstop against destructive operations from
  a cluster-admin is **admission policy** (ValidatingAdmissionPolicy / Kyverno), not this tool.

So, plainly: tessera is primarily an **accident-limiter for the thing you point it at**, and a
**real security boundary only when the operator is non-admin**. It does not — and cannot — contain a
malicious admin. Do not deploy it believing otherwise.

## What it is

`kubectl tessera` mints a short-lived, least-privilege Kubernetes session in one shot:

1. **SSAR pre-flight gate** — before creating anything, it runs a `SelfSubjectAccessReview` for the
   requested verbs/resources. If you can't do the thing yourself, it fails fast instead of creating
   orphaned RBAC objects you'll have to chase down.
2. **Mint** — it creates a narrowly scoped Role/ClusterRole and a binding to your identity, then
   requests a bound, TTL-limited token via the TokenRequest API. All created objects carry tessera's
   labels (`tessera.adustio.com/*`) so they can be swept later.
3. **Emit** — it produces a throwaway kubeconfig pointing at the same cluster, authenticated by the
   minted token, with nothing written to your real `~/.kube/config`.
4. **Auto-cleanup** — depending on mode, the session either tears its own RBAC objects down on exit
   (`--exec`) or leaves them labeled for the sweeper (`--print-kubeconfig` + `tessera gc`).

## Install

Via krew (pending krew-index acceptance):

```bash
kubectl krew install tessera
```

Or directly with Go:

```bash
go install github.com/wagneripjr/kubectl-tessera/cmd/kubectl-tessera@latest
```

Either way, the binary is `kubectl-tessera`, invoked as `kubectl tessera`.

## Usage

```bash
kubectl tessera --resource pods [flags]
```

| Flag | Default | Meaning |
|------|---------|---------|
| `--verb` | `get,list,watch` | Comma-separated verbs to grant. |
| `--resource` | *(required)* | Comma-separated resources to scope to (e.g. `pods,deployments`), or `'*'` for **all resources** (explicit opt-in; the SSAR pre-flight makes this admin-only, so it can't escalate you). |
| `-n`, `--namespace` | *(current context)* | Namespace to scope to. Omit for a cluster-scoped grant. |
| `--ttl` | `15m` | Token lifetime. The API server auto-revokes after this. |
| `--resource-name` | *(none)* | Restrict the grant to named resource instances. |
| `--api-group` | *(core)* | API group of the target resource(s). |
| `--cluster` | *(current context)* | Cluster/context to mint against. |
| `--exec` | *(default)* | Drop into an interactive subshell with the scoped kubeconfig; clean up RBAC on exit. |
| `--print-kubeconfig` | | Print the kubeconfig to stdout (for agents/automation). Leaves RBAC objects for `gc`. |
| `--dry-run` | | Show what would be created without creating it. |
| `-o json` | | Machine-readable output. |

`--exec` and `--print-kubeconfig` are **mutually exclusive** — one drives an interactive session
that cleans up after itself, the other hands the kubeconfig to something else and relies on the
sweeper.

### Subcommands

| Command | Purpose |
|---------|---------|
| `tessera gc` | Sweep and delete expired/orphaned tessera RBAC objects by label. |
| `tessera ls` | List active tessera-minted sessions and their RBAC objects. |

## Claude Code integration

The intended pattern: mint a read-only, auto-expiring kubeconfig and hand it to an agent. Use
`--print-kubeconfig` so the agent gets a self-contained kubeconfig and nothing touches your real
config:

```bash
export KUBECONFIG="$(kubectl tessera \
  --verb get,list,watch \
  --resource pods,deployments,events,replicasets \
  --namespace prod --ttl 1h --print-kubeconfig)"
claude   # read-only in prod, auto-expiring in 1h
```

For an agent that must **read across every resource type** rather than a fixed list, use the `'*'`
wildcard (quote it so the shell doesn't glob-expand it). With the read-only default verbs and `-A`,
this mints an ephemeral cluster-wide reader — and the SSAR pre-flight still refuses it unless you
already hold cluster-wide read, so it can't escalate you:

```bash
export KUBECONFIG="$(kubectl tessera \
  --resource '*' --verb get,list,watch \
  --all-namespaces --ttl 1h --print-kubeconfig)"
claude   # read everything, everywhere, read-only, for 1h
```

Because `--print-kubeconfig` leaves the RBAC objects behind for the agent's lifetime, **something
must reclaim them**: run `tessera gc` after the session, or schedule the in-cluster CronJob (see
below). The token itself expires on its own via TTL; `gc` is what cleans up the Role/binding.

## Cleanup model

Three independent layers, so a failure in one doesn't strand objects forever:

1. **Token TTL (API-server enforced).** The bound token is requested with `--ttl`; the API server
   stops honoring it after that, regardless of what happens to the client. This is the one layer you
   can't bypass.
2. **`--exec` foreground signal trap.** In interactive mode, tessera traps SIGINT/SIGTERM and
   deletes the RBAC objects it created when the subshell exits cleanly. This is the fast path — the
   objects are gone seconds after you `exit`.
3. **`tessera gc` label sweep.** Required for `--print-kubeconfig`, and the backstop for everything
   else. A `SIGKILL` bypasses the signal trap entirely (you can't trap `SIGKILL`), so the Role and
   binding would otherwise linger; `gc` finds them by their `tessera.adustio.com/*` labels and
   reclaims them. Run it from cron, a CI step, or the bundled in-cluster CronJob.

## Status

Early, pre-1.0, scaffolding stage. The CLI surface and label schema documented here are the target;
expect the implementation to lag the docs while the walking skeleton and ATDD harness come together.
Don't depend on it for anything that matters yet.

## License

Apache-2.0. See [LICENSE](LICENSE).
