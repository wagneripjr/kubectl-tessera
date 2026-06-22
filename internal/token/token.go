// Package token mints short-lived ServiceAccount tokens via the TokenRequest API,
// surfacing the returned (possibly clamped) ExpirationTimestamp. See FR-006.
package token

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

// minServerMinor is the first Kubernetes minor (in the 1.x line) to ship the TokenRequest
// API as GA. Below this, tessera cannot mint and must say so plainly (FR-016).
const minServerMinor = 24

// RequireSupported fails clearly when the target cluster predates the TokenRequest API
// (Kubernetes < 1.24). The returned error's message carries the literal FR-016 contract so
// the operator gets an actionable line, not an opaque 404 deep in the mint. The minor
// version may carry a non-numeric suffix on managed distributions (e.g. GKE's "27+"), so
// only the leading digits are parsed.
func RequireSupported(disco discovery.ServerVersionInterface) error {
	info, err := disco.ServerVersion()
	if err != nil {
		return fmt.Errorf("discovering server version: %w", err)
	}
	major := leadingInt(info.Major)
	minor := leadingInt(info.Minor)
	if major < 1 || (major == 1 && minor < minServerMinor) {
		return fmt.Errorf("server is Kubernetes %s.%s: tessera requires Kubernetes >= 1.24 (TokenRequest API)", info.Major, info.Minor)
	}
	return nil
}

// leadingInt parses the leading run of digits in s (e.g. "27+" -> 27, "" -> 0).
func leadingInt(s string) int {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	n, _ := strconv.Atoi(strings.TrimSpace(s[:end]))
	return n
}

// now is the package clock, overridable in tests for deterministic clamp checks.
var now = time.Now

// clampSkew tolerates small differences between requested and returned expiry
// (clock skew, rounding) before reporting the token as clamped.
const clampSkew = 10 * time.Second

// MinTTL is the kube-apiserver's hardcoded TokenRequest minimum: ValidateTokenRequest
// rejects any ExpirationSeconds below 10 minutes with a 422. The value is not
// configurable or discoverable via the API, so we floor sub-minimum requests up to it
// rather than letting the mint fail (FR-006).
const MinTTL = 10 * time.Minute

// Minted is the result of a successful TokenRequest.
type Minted struct {
	Token               string
	ExpirationTimestamp time.Time
	// Clamped is true when the cluster returned a shorter lifetime than requested
	// (e.g. --service-account-max-token-expiration), beyond clampSkew.
	Clamped bool
	// Floored is true when the requested TTL was below MinTTL and was raised to it
	// before the request, so the cluster would accept it.
	Floored bool
}

// Mint requests a bound token for saName via the TokenRequest API using cs (the
// invoking user's clientset). The ServiceAccount must already exist. A TTL below MinTTL
// is floored up to it (the cluster would otherwise reject the request); the returned
// ExpirationTimestamp is authoritative — it reflects any cluster-side clamping.
func Mint(ctx context.Context, cs kubernetes.Interface, ns, saName string, ttl time.Duration) (Minted, error) {
	effective := ttl
	floored := false
	if effective < MinTTL {
		effective = MinTTL
		floored = true
	}
	tr := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: ptr.To(int64(effective.Seconds()))},
	}
	out, err := cs.CoreV1().ServiceAccounts(ns).CreateToken(ctx, saName, tr, metav1.CreateOptions{})
	if err != nil {
		return Minted{}, fmt.Errorf("requesting token for %s/%s: %w", ns, saName, err)
	}
	expiry := out.Status.ExpirationTimestamp.Time
	clamped := expiry.Before(now().Add(effective).Add(-clampSkew))
	return Minted{Token: out.Status.Token, ExpirationTimestamp: expiry, Clamped: clamped, Floored: floored}, nil
}
