// Package token mints short-lived ServiceAccount tokens via the TokenRequest API,
// surfacing the returned (possibly clamped) ExpirationTimestamp. See FR-006.
package token

import "errors"

// ErrNotImplemented marks behavior not yet built in the walking skeleton.
var ErrNotImplemented = errors.New("token: not implemented")
