package token

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

// fakeServerVersion satisfies discovery.ServerVersionInterface with a canned version
// (or a forced error), so RequireSupported is testable without a live API server.
type fakeServerVersion struct {
	info *version.Info
	err  error
}

func (f fakeServerVersion) ServerVersion() (*version.Info, error) { return f.info, f.err }

func TestRequireSupported(t *testing.T) {
	cases := []struct {
		name    string
		major   string
		minor   string
		wantErr bool
	}{
		{name: "1.23 is too old", major: "1", minor: "23", wantErr: true},
		{name: "exactly 1.24 is supported", major: "1", minor: "24", wantErr: false},
		{name: "1.34 is supported", major: "1", minor: "34", wantErr: false},
		{name: "managed-distro suffix 27+ parses as supported", major: "1", minor: "27+", wantErr: false},
		{name: "managed-distro suffix 23+ parses as too old", major: "1", minor: "23+", wantErr: true},
		{name: "future major is supported", major: "2", minor: "0", wantErr: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := RequireSupported(fakeServerVersion{info: &version.Info{Major: tc.major, Minor: tc.minor}})
			if (err != nil) != tc.wantErr {
				t.Fatalf("RequireSupported(%s.%s) error = %v, wantErr = %v", tc.major, tc.minor, err, tc.wantErr)
			}
			if tc.wantErr && !strings.Contains(err.Error(), "requires Kubernetes >= 1.24 (TokenRequest API)") {
				t.Errorf("expected the FR-016 literal in the error, got: %v", err)
			}
		})
	}
}

func TestRequireSupportedPropagatesDiscoveryError(t *testing.T) {
	err := RequireSupported(fakeServerVersion{err: fmt.Errorf("connection refused")})
	if err == nil {
		t.Fatal("expected RequireSupported to propagate a discovery failure")
	}
}

var fixedNow = time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)

// swapNow pins the package clock for deterministic clamp assertions.
func swapNow(t *testing.T, at time.Time) {
	t.Helper()
	prev := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = prev })
}

// tokenReactor intercepts the serviceaccounts/token subresource create and returns
// a canned TokenRequest status, so the test does not depend on fake CreateToken support.
func tokenReactor(token string, expiry time.Time) clienttesting.ReactionFunc {
	return func(action clienttesting.Action) (bool, runtime.Object, error) {
		create := action.(clienttesting.CreateAction)
		if create.GetSubresource() != "token" {
			return false, nil, nil
		}
		tr := create.GetObject().(*authenticationv1.TokenRequest).DeepCopy()
		tr.Status.Token = token
		tr.Status.ExpirationTimestamp = metav1.NewTime(expiry)
		return true, tr, nil
	}
}

func mintWith(t *testing.T, token string, expiry time.Time) Minted {
	t.Helper()
	cs := fake.NewSimpleClientset()
	cs.PrependReactor("create", "serviceaccounts", tokenReactor(token, expiry))
	swapNow(t, fixedNow)
	m, err := Mint(context.Background(), cs, "prod", "sa", 15*time.Minute)
	if err != nil {
		t.Fatalf("Mint returned error: %v", err)
	}
	return m
}

func TestMintReturnsTokenAndReturnedExpiry(t *testing.T) {
	exp := fixedNow.Add(15 * time.Minute)
	m := mintWith(t, "minted-xyz", exp)
	if m.Token != "minted-xyz" {
		t.Fatalf("token = %q, want minted-xyz", m.Token)
	}
	if !m.ExpirationTimestamp.Equal(exp) {
		t.Fatalf("expiry = %v, want %v", m.ExpirationTimestamp, exp)
	}
}

func TestMintDetectsClamping(t *testing.T) {
	cases := []struct {
		name        string
		expiry      time.Time
		wantClamped bool
	}{
		{name: "full ttl honored", expiry: fixedNow.Add(15 * time.Minute), wantClamped: false},
		{name: "ttl shortened by the cluster", expiry: fixedNow.Add(2 * time.Minute), wantClamped: true},
		{name: "sub-skew difference is tolerated", expiry: fixedNow.Add(15*time.Minute - 5*time.Second), wantClamped: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := mintWith(t, "minted-xyz", tc.expiry)
			if m.Clamped != tc.wantClamped {
				t.Fatalf("Clamped = %t, want %t", m.Clamped, tc.wantClamped)
			}
		})
	}
}

// echoExpiryReactor models a cluster that honors the requested lifetime: it returns a
// token expiring fixedNow + the requested ExpirationSeconds. Tests then assert on Mint's
// public result (ExpirationTimestamp, Floored) rather than the request it sent.
func echoExpiryReactor(token string) clienttesting.ReactionFunc {
	return func(action clienttesting.Action) (bool, runtime.Object, error) {
		create := action.(clienttesting.CreateAction)
		if create.GetSubresource() != "token" {
			return false, nil, nil
		}
		tr := create.GetObject().(*authenticationv1.TokenRequest).DeepCopy()
		tr.Status.Token = token
		tr.Status.ExpirationTimestamp = metav1.NewTime(fixedNow.Add(time.Duration(*tr.Spec.ExpirationSeconds) * time.Second))
		return true, tr, nil
	}
}

func TestMintFloorsSubMinimumTTL(t *testing.T) {
	cases := []struct {
		name          string
		ttl           time.Duration
		wantEffective time.Duration
		wantFloored   bool
	}{
		{name: "below the minimum is floored up to it", ttl: 30 * time.Second, wantEffective: MinTTL, wantFloored: true},
		{name: "exactly the minimum is left alone", ttl: MinTTL, wantEffective: MinTTL, wantFloored: false},
		{name: "above the minimum is honored", ttl: 15 * time.Minute, wantEffective: 15 * time.Minute, wantFloored: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs := fake.NewSimpleClientset()
			cs.PrependReactor("create", "serviceaccounts", echoExpiryReactor("minted-xyz"))
			swapNow(t, fixedNow)

			m, err := Mint(context.Background(), cs, "prod", "sa", tc.ttl)
			if err != nil {
				t.Fatalf("Mint returned error: %v", err)
			}
			if want := fixedNow.Add(tc.wantEffective); !m.ExpirationTimestamp.Equal(want) {
				t.Fatalf("effective expiry = %v, want %v (effective ttl %s)", m.ExpirationTimestamp, want, tc.wantEffective)
			}
			if m.Floored != tc.wantFloored {
				t.Fatalf("Floored = %t, want %t", m.Floored, tc.wantFloored)
			}
		})
	}
}

func TestMintPropagatesError(t *testing.T) {
	cs := fake.NewSimpleClientset()
	cs.PrependReactor("create", "serviceaccounts", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.(clienttesting.CreateAction).GetSubresource() == "token" {
			return true, nil, fmt.Errorf("forbidden")
		}
		return false, nil, nil
	})

	_, err := Mint(context.Background(), cs, "prod", "sa", 15*time.Minute)
	if err == nil {
		t.Fatal("expected Mint to propagate the token request error")
	}
}
