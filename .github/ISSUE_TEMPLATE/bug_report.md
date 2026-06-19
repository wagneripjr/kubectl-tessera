---
name: Bug report
about: Report a defect in kubectl-tessera
title: "[BUG] "
labels: bug
assignees: ''
---

## Summary

A clear, concise description of the bug.

## Steps to reproduce

The exact `kubectl tessera …` command(s) and any cluster setup.

```
kubectl tessera --verb ... --resource ... --namespace ...
```

## Expected behavior

What you expected to happen.

## Actual behavior

What actually happened. Include stderr (the audit/diagnostic stream) — **never paste a real
token or kubeconfig contents**.

## Environment

- tessera version (`kubectl tessera version`):
- kubectl version (`kubectl version --short`):
- Kubernetes server version:
- **Are you cluster-admin on this cluster?** (yes/no — relevant to the trust model; the tool is
  a real boundary only for non-admins)
- OS / arch:

## Additional context

Anything else relevant (RBAC setup, authorizer type, etc.).
