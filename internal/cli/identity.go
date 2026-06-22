package cli

import (
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/validation"
)

const dns1123MaxLabel = 63

var nonDNS1123 = regexp.MustCompile(`[^a-z0-9]+`)

func sanitizeDNS1123Raw(s string) string {
	s = strings.ToLower(s)
	s = nonDNS1123.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func sanitizeDNS1123(s string) string {
	if out := sanitizeDNS1123Raw(s); out != "" {
		return out
	}
	return "user"
}

func baseName(owner, sessionID string) string {
	owner = sanitizeDNS1123Raw(owner)

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

func newSessionID() string {
	return strings.ToLower(string(uuid.NewUUID()))[:8]
}
