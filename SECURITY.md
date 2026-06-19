# Security Policy

## Reporting a vulnerability

**Do not open a public GitHub issue for security vulnerabilities.** Public issues are visible to
everyone and disclose the problem before a fix exists.

Email **wagneripjr@adustio.com** with:

- a description of the vulnerability and its impact,
- steps to reproduce (or a proof of concept),
- affected version(s) / commit, and
- any suggested remediation if you have one.

**Expected response window:** an acknowledgement within **3 business days**, and an initial
assessment (severity, whether it's accepted) within **10 business days**. Because this is a
pre-1.0, single-maintainer project, fix timelines are negotiated case by case once the report is
triaged. Please allow a reasonable embargo period before public disclosure; coordinated disclosure
is appreciated.

## Supported versions

This project is **pre-1.0**. Only the latest tagged release (and `master`) receives security fixes.
There are no maintained release branches yet.

| Version | Supported |
|---------|-----------|
| `master` (unreleased) | ✅ |
| latest `0.x` tag | ✅ |
| any earlier `0.x` tag | ❌ |

Once a `1.0.0` is cut, this table will be revised to state a real support window.

## Trust model

Read this before relying on tessera as a control. It restates the caveat from the README because
it is the single most important property to understand correctly.

**What tessera guarantees, unconditionally:**

- The minted token is **always scoped** to exactly the verbs/resources/namespace you requested. The
  bearer of that token cannot exceed that scope.
- Tessera **never writes the minted token to `~/.kube/config`** and **never passes it on the command
  line / `argv`** (where it would leak into process listings and shell history). The token lives in
  a throwaway kubeconfig.
- The emitted kubeconfig file is created with `0600` permissions (owner read/write only).
- A pre-flight `SelfSubjectAccessReview` (SSAR) ensures tessera only attempts to mint what the
  operator can already do — it fails fast rather than leaving orphaned RBAC behind.

**What tessera does NOT guarantee — and what determines whether it's a real boundary:**

- **For a non-cluster-admin operator:** tessera is a **real security boundary**. Kubernetes RBAC
  escalation-prevention forbids binding to rules you don't hold (absent the `bind` verb), so you
  cannot mint a token broader than your own privilege.
- **For a cluster-admin operator:** escalation-prevention is vacuous, so tessera **cannot prevent
  over-scoping**. It is still a useful self-imposed blast-radius guardrail for whatever consumes the
  token (an agent, a job, a teammate), but it is **not a containment control on a privileged human**.
  The hard backstop against destructive operations in that case is **admission policy**
  (ValidatingAdmissionPolicy / Kyverno), not tessera.

In short: tessera mints credentials and limits blast radius; it is a containment boundary only when
the operator is not already cluster-admin. It is never a defense against a malicious administrator.
