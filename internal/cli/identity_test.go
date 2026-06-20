package cli

import (
	"regexp"
	"testing"

	"k8s.io/apimachinery/pkg/util/validation"
)

func TestSanitizeDNS1123(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"already valid", "alice", "alice"},
		{"uppercase lowered", "Alice", "alice"},
		{"colons become dashes", "system:serviceaccount:prod:ci", "system-serviceaccount-prod-ci"},
		{"collapse repeated separators", "a__b..c", "a-b-c"},
		{"trim leading and trailing separators", "-alice-", "alice"},
		{"email-ish", "alice@example.com", "alice-example-com"},
		{"empty falls back to user", "", "user"},
		{"only separators falls back to user", "@@@", "user"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeDNS1123(tc.in)
			if got != tc.want {
				t.Fatalf("sanitizeDNS1123(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if errs := validation.IsDNS1123Label(got); len(errs) != 0 {
				t.Fatalf("sanitizeDNS1123(%q) = %q is not a valid DNS-1123 label: %v", tc.in, got, errs)
			}
		})
	}
}

func TestBaseName(t *testing.T) {
	t.Run("composes tessera owner and session id", func(t *testing.T) {
		got := baseName("alice", "1a2b3c4d")
		if got != "tessera-alice-1a2b3c4d" {
			t.Fatalf("baseName = %q, want tessera-alice-1a2b3c4d", got)
		}
	})

	t.Run("result is always a valid DNS-1123 label within 63 chars", func(t *testing.T) {
		longOwner := "system-serviceaccount-a-very-long-namespace-name-and-account-identifier"
		got := baseName(longOwner, "1a2b3c4d")
		if len(got) > 63 {
			t.Fatalf("baseName length = %d, want <= 63 (%q)", len(got), got)
		}
		if errs := validation.IsDNS1123Label(got); len(errs) != 0 {
			t.Fatalf("baseName = %q is not a valid DNS-1123 label: %v", got, errs)
		}
	})

	t.Run("falls back to session id when owner is unusable", func(t *testing.T) {
		got := baseName("", "1a2b3c4d")
		if got != "tessera-1a2b3c4d" {
			t.Fatalf("baseName with empty owner = %q, want tessera-1a2b3c4d", got)
		}
	})
}

var sessionIDFormat = regexp.MustCompile(`^[a-z0-9]{8}$`)

func TestNewSessionID(t *testing.T) {
	t.Run("is eight lowercase hex characters", func(t *testing.T) {
		id := newSessionID()
		if !sessionIDFormat.MatchString(id) {
			t.Fatalf("newSessionID() = %q, want 8 lowercase hex chars", id)
		}
	})

	t.Run("is distinct across calls", func(t *testing.T) {
		first := newSessionID()
		second := newSessionID()
		if first == second {
			t.Fatalf("newSessionID() returned the same value twice: %q", first)
		}
	})
}
