// Package rbac creates the managed ServiceAccount, (Cluster)Role and
// (Cluster)RoleBinding set as the invoking user, with reverse-order rollback on
// partial failure. Creation must never use a privileged context or impersonation
// (NFR-002, ADR-005). See FR-004, FR-005.
package rbac

import "errors"

// ErrNotImplemented marks behavior not yet built in the walking skeleton.
var ErrNotImplemented = errors.New("rbac: not implemented")
