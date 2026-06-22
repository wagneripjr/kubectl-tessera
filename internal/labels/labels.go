package labels

import "fmt"

const (
	Domain = "tessera.adustio.com"

	ManagedByKey   = "app.kubernetes.io/managed-by"
	ManagedByValue = "kubectl-tessera"

	OwnerKey = Domain + "/owner"

	SessionIDKey = Domain + "/session-id"

	ExpiresAtKey = Domain + "/expires-at"
)

func ManagedSelector() string {
	return fmt.Sprintf("%s=%s", ManagedByKey, ManagedByValue)
}

func SessionSelector(sessionID string) string {
	return fmt.Sprintf("%s=%s,%s=%s", ManagedByKey, ManagedByValue, SessionIDKey, sessionID)
}

func Set(owner, sessionID string) map[string]string {
	return map[string]string{
		ManagedByKey: ManagedByValue,
		OwnerKey:     owner,
		SessionIDKey: sessionID,
	}
}
