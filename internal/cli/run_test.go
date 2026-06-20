package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/wagneripjr/kubectl-tessera/internal/preflight"
	"github.com/wagneripjr/kubectl-tessera/internal/scope"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		opts    mintOptions
		wantErr bool
	}{
		{name: "missing resource is rejected", opts: mintOptions{printKubeconfig: true}, wantErr: true},
		{name: "print-kubeconfig and exec together are rejected", opts: mintOptions{resources: []string{"pods"}, printKubeconfig: true, exec: true}, wantErr: true},
		{name: "print-kubeconfig with a resource is accepted", opts: mintOptions{resources: []string{"pods"}, printKubeconfig: true}, wantErr: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected a validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestBuildAttributes(t *testing.T) {
	cases := []struct {
		name      string
		resources []scope.ResolvedResource
		verbs     []string
		names     []string
		namespace string
		wantCount int
		check     func(t *testing.T, attrs []preflight.Attribute)
	}{
		{
			name:      "one attribute per verb for a namespaced resource",
			resources: []scope.ResolvedResource{{Resource: "pods", Group: "", Namespaced: true}},
			verbs:     []string{"get", "list"},
			namespace: "prod",
			wantCount: 2,
			check: func(t *testing.T, attrs []preflight.Attribute) {
				for _, a := range attrs {
					if a.Resource != "pods" || a.Namespace != "prod" {
						t.Fatalf("attr = %+v, want pods in prod", a)
					}
				}
			},
		},
		{
			name:      "namespace cleared for a cluster-scoped resource",
			resources: []scope.ResolvedResource{{Resource: "nodes", Group: "", Namespaced: false}},
			verbs:     []string{"get"},
			namespace: "prod",
			wantCount: 1,
			check: func(t *testing.T, attrs []preflight.Attribute) {
				if attrs[0].Namespace != "" {
					t.Fatalf("cluster-scoped attribute namespace = %q, want empty", attrs[0].Namespace)
				}
			},
		},
		{
			name:      "one attribute per resource name",
			resources: []scope.ResolvedResource{{Resource: "pods", Group: "", Namespaced: true}},
			verbs:     []string{"get"},
			names:     []string{"foo", "bar"},
			namespace: "prod",
			wantCount: 2,
			check: func(t *testing.T, attrs []preflight.Attribute) {
				if attrs[0].Name != "foo" || attrs[1].Name != "bar" {
					t.Fatalf("names = %q,%q, want foo,bar", attrs[0].Name, attrs[1].Name)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			attrs := buildAttributes(tc.resources, tc.verbs, tc.names, tc.namespace)
			if len(attrs) != tc.wantCount {
				t.Fatalf("attrs = %+v, want %d", attrs, tc.wantCount)
			}
			tc.check(t, attrs)
		})
	}
}

func TestAuditLine(t *testing.T) {
	expires := time.Date(2026, 6, 19, 12, 15, 0, 0, time.UTC)
	cases := []struct {
		name          string
		namespace     string
		clusterScoped bool
		wantParts     []string
	}{
		{
			name:      "namespaced session carries the session-id token the driver parses",
			namespace: "prod",
			wantParts: []string{"session-id=1a2b3c4d", "owner=alice", "ns=prod", "ttl=15m", "expires=2026-06-19T12:15:00Z"},
		},
		{
			name:          "cluster-scoped session reports ns=cluster",
			clusterScoped: true,
			wantParts:     []string{"session-id=1a2b3c4d", "ns=cluster", "cluster-scoped=true"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			line := auditLine("1a2b3c4d", "alice", []string{"get", "list"}, []string{"pods"}, nil, tc.namespace, 15*time.Minute, expires, tc.clusterScoped)
			for _, want := range tc.wantParts {
				if !strings.Contains(line, want) {
					t.Fatalf("audit line missing %q: %q", want, line)
				}
			}
		})
	}
}
