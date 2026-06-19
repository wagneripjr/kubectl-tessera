// Package preflight implements the SelfSubjectAccessReview authorization gate
// (authoritative, FR-003) and SelfSubjectRulesReview discovery (advisory only,
// honoring the Incomplete flag, FR-013). See ADR-006.
package preflight

import "errors"

// ErrNotImplemented marks behavior not yet built in the walking skeleton.
var ErrNotImplemented = errors.New("preflight: not implemented")
