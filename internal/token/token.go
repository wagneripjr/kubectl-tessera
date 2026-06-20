// Package token mints short-lived ServiceAccount tokens via the TokenRequest API,
// surfacing the returned (possibly clamped) ExpirationTimestamp. See FR-006.
package token

import (
	"context"
	"fmt"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

// now is the package clock, overridable in tests for deterministic clamp checks.
var now = time.Now

// clampSkew tolerates small differences between requested and returned expiry
// (clock skew, rounding) before reporting the token as clamped.
const clampSkew = 10 * time.Second

// Minted is the result of a successful TokenRequest.
type Minted struct {
	Token               string
	ExpirationTimestamp time.Time
	// Clamped is true when the cluster returned a shorter lifetime than requested
	// (e.g. --service-account-max-token-expiration), beyond clampSkew.
	Clamped bool
}

// Mint requests a bound token for saName via the TokenRequest API using cs (the
// invoking user's clientset). The ServiceAccount must already exist. The returned
// ExpirationTimestamp is authoritative — it reflects any cluster-side clamping.
func Mint(ctx context.Context, cs kubernetes.Interface, ns, saName string, ttl time.Duration) (Minted, error) {
	tr := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: ptr.To(int64(ttl.Seconds()))},
	}
	out, err := cs.CoreV1().ServiceAccounts(ns).CreateToken(ctx, saName, tr, metav1.CreateOptions{})
	if err != nil {
		return Minted{}, fmt.Errorf("requesting token for %s/%s: %w", ns, saName, err)
	}
	expiry := out.Status.ExpirationTimestamp.Time
	clamped := expiry.Before(now().Add(ttl).Add(-clampSkew))
	return Minted{Token: out.Status.Token, ExpirationTimestamp: expiry, Clamped: clamped}, nil
}
