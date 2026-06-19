// Package gc sweeps expired managed RBAC object sets, selected by the
// app.kubernetes.io/managed-by=kubectl-tessera label and the expires-at annotation.
// It is idempotent and cron-safe (FR-011, NFR-005). See ADR-007.
package gc

import "errors"

// ErrNotImplemented marks behavior not yet built in the walking skeleton.
var ErrNotImplemented = errors.New("gc: not implemented")
