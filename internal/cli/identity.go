package cli

import (
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/validation"
)

// dns1123MaxLabel is the Kubernetes DNS-1123 label length limit. Object names
// derived from owner + session id must stay within it.
const dns1123MaxLabel = 63

var nonDNS1123 = regexp.MustCompile(`[^a-z0-9]+`)

// sanitizeDNS1123Raw lowercases, collapses runs of non-[a-z0-9] to a single dash,
// and trims leading/trailing dashes. It returns "" when nothing usable remains —
// callers decide how to handle the empty case.
func sanitizeDNS1123Raw(s string) string {
	s = strings.ToLower(s)
	s = nonDNS1123.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// sanitizeDNS1123 maps an arbitrary identity string to a valid DNS-1123 label,
// falling back to "user" when nothing usable remains so the owner label value is
// always populated.
func sanitizeDNS1123(s string) string {
	if out := sanitizeDNS1123Raw(s); out != "" {
		return out
	}
	return "user"
}

// baseName builds the shared object name "tessera-<owner>-<sessionID>", truncating
// the sanitized owner as needed so the result is a valid DNS-1123 label within the
// length limit. If the owner cannot be made to fit, it is dropped and the name
// falls back to "tessera-<sessionID>".
func baseName(owner, sessionID string) string {
	owner = sanitizeDNS1123Raw(owner)
	// Budget for the owner: 63 - len("tessera-") - len("-") - len(sessionID).
	budget := dns1123MaxLabel - len("tessera-") - len("-") - len(sessionID)
	if owner != "" && budget >= 1 {
		if len(owner) > budget {
			owner = strings.Trim(owner[:budget], "-")
		}
		candidate := "tessera-" + owner + "-" + sessionID
		if owner != "" && len(validation.IsDNS1123Label(candidate)) == 0 {
			return candidate
		}
	}
	return "tessera-" + sessionID
}

// newSessionID returns the first 8 lowercase hex characters of a UUID. The short
// id ties every object created by one mint together and appears in the audit line.
func newSessionID() string {
	return strings.ToLower(string(uuid.NewUUID()))[:8]
}
