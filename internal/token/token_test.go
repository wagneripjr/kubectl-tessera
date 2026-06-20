package token

import (
	"context"
	"fmt"
	"testing"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

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
