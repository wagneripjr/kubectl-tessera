// Package kubeconfig builds an isolated, 0600 throwaway kubeconfig containing only
// the minted token, the source cluster's server+CA, and a context bound to the
// target namespace. It never touches the user's ~/.kube/config. See FR-007, NFR-001.
package kubeconfig

import "errors"

// ErrNotImplemented marks behavior not yet built in the walking skeleton.
var ErrNotImplemented = errors.New("kubeconfig: not implemented")
