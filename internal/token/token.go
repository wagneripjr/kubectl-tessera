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

const minServerMinor = 24

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

func leadingInt(s string) int {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	n, _ := strconv.Atoi(strings.TrimSpace(s[:end]))
	return n
}

var now = time.Now

const clampSkew = 10 * time.Second

const MinTTL = 10 * time.Minute

type Minted struct {
	Token               string
	ExpirationTimestamp time.Time

	Clamped bool

	Floored bool
}

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
