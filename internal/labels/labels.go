// Package labels defines the label and annotation keys tessera stamps on every
// managed object, plus helpers for building selectors and label sets. These keys
// are the contract that gc, ls, and cleanup rely on (see ADR-008).
package labels

import "fmt"

const (
	// Domain is the DNS label/annotation namespace owned by tessera.
	Domain = "tessera.adustio.com"

	// ManagedByKey and ManagedByValue mark every object tessera creates. The
	// managed-by selector is the safety filter: gc/ls/cleanup only ever touch
	// objects carrying it.
	ManagedByKey   = "app.kubernetes.io/managed-by"
	ManagedByValue = "kubectl-tessera"

	// OwnerKey labels an object with the sanitized invoking-user identity.
	OwnerKey = Domain + "/owner"
	// SessionIDKey ties every object created by one mint to a single session.
	SessionIDKey = Domain + "/session-id"
	// ExpiresAtKey annotates an object with its RFC3339 UTC expiry; gc reads it.
	ExpiresAtKey = Domain + "/expires-at"
)

// ManagedSelector is the label selector matching every tessera-managed object.
func ManagedSelector() string {
	return fmt.Sprintf("%s=%s", ManagedByKey, ManagedByValue)
}

// SessionSelector matches every object belonging to a single session.
func SessionSelector(sessionID string) string {
	return fmt.Sprintf("%s=%s,%s=%s", ManagedByKey, ManagedByValue, SessionIDKey, sessionID)
}

// Set returns the standard label set stamped on every managed object.
func Set(owner, sessionID string) map[string]string {
	return map[string]string{
		ManagedByKey: ManagedByValue,
		OwnerKey:     owner,
		SessionIDKey: sessionID,
	}
}
