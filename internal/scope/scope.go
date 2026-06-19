// Package scope parses a requested credential scope and resolves each resource to
// its GVR/GVK and namespaced-vs-cluster scope via a discovery RESTMapper, producing
// concrete RBAC PolicyRules. See docs/requirements/minting.md (FR-001, FR-002).
package scope

import "errors"

// ErrNotImplemented marks behavior not yet built in the walking skeleton.
var ErrNotImplemented = errors.New("scope: not implemented")
