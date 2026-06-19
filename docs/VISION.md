# Vision — kubectl-tessera

## Purpose

`kubectl-tessera` is a kubectl plugin (invoked as `kubectl tessera`) that issues an
**ephemeral, scope-narrowed, TTL-bound** credential for the current cluster. It runs
**as the invoking user**, performs a `SelfSubjectAccessReview` pre-flight, and
**automatically cleans up** the RBAC objects it creates. The output is a throwaway
kubeconfig you point a client (Claude Code, or any kubectl-compatible tool) at for a
time-boxed, least-privilege session.

## Target users

- **Operators/SREs running AI coding agents** (e.g. Claude Code) against live clusters
  who want the agent confined to a narrow scope that self-expires.
- **Engineers** who want a quick, disposable least-privilege kubeconfig without editing
  RBAC by hand and remembering to tear it down.
- **Incident responders** needing a short, deliberately-widened, object-pinned write
  window with an automatic expiry.

## What this tool actually buys you (the cluster-admin caveat — load-bearing)

The minted token is **always** narrow — whoever uses it is confined to the requested
scope. What changes is the guarantee around the *operator's own* privilege:

- **If the operator is NOT cluster-admin:** Kubernetes RBAC escalation-prevention makes
  this a **real security boundary**. You physically cannot mint a token with more than
  you already hold — the API server rejects creation of a binding to a role whose rules
  you don't possess (unless you hold the `bind` verb on it).
- **If the operator IS cluster-admin:** escalation-prevention is vacuous, so the tool
  cannot stop you from over-scoping. It remains an effective **self-imposed blast-radius
  guardrail for the agent**, but it is *not* a containment control on a privileged human.
  The hard backstop against destructive operations stays in **admission policy**
  (ValidatingAdmissionPolicy / Kyverno), not here.

The tool is primarily an **accident-limiter for the agent**, and a real security boundary
only for non-admin operators. This must be stated plainly at the top of the README and
must never be contradicted by a claim of containment.

## Success criteria

1. The 11 end-to-end acceptance criteria (see `docs/TRACEABILITY.md` and
   `specs/features/`) pass on a kind cluster.
2. A non-admin operator provably cannot mint a credential exceeding their own privilege
   (the API server enforces this at binding creation, not just our pre-flight gate).
3. No token ever reaches `~/.kube/config`, process argv, or shell history; the throwaway
   kubeconfig is `0600`.
4. Every session is reclaimable: token TTL, `--exec` signal trap, and `tessera gc` form a
   three-layer cleanup with no orphaned objects.
5. Installable via `kubectl krew install tessera` (post krew-index acceptance) and
   `go install`.

## Non-goals

- **Not** an admission controller and **not** a containment control against a malicious or
  mistaken cluster-admin. Admission policy owns that.
- **Not** a replacement for cluster RBAC design or for short-lived OIDC/IRSA identities.
- **Not** a secrets manager — it mints Kubernetes ServiceAccount tokens only.
