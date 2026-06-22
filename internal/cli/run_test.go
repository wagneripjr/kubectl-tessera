package cli

import (
	"strings"
	"testing"
	"time"

	"k8s.io/cli-runtime/pkg/genericclioptions"

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

func TestValidateOutputFlag(t *testing.T) {
	cases := []struct {
		name    string
		opts    mintOptions
		wantErr bool
	}{
		{name: "json output is accepted", opts: mintOptions{resources: []string{"pods"}, output: "json"}, wantErr: false},
		{name: "unsupported output format is rejected", opts: mintOptions{resources: []string{"pods"}, output: "yaml"}, wantErr: true},
		{name: "json output with exec is rejected (cannot stream json into a subshell)", opts: mintOptions{resources: []string{"pods"}, output: "json", exec: true}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.opts.validate(); (err != nil) != tc.wantErr {
				t.Fatalf("validate() error = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestBuildCreateAttributes(t *testing.T) {
	t.Run("namespaced set needs create on serviceaccounts/roles/rolebindings in the namespace", func(t *testing.T) {
		attrs := buildCreateAttributes(false, "prod", []string{"prod"})
		got := map[string]string{} // resource -> namespace
		for _, a := range attrs {
			if a.Verb != "create" {
				t.Fatalf("attr verb = %q, want create", a.Verb)
			}
			got[a.Resource] = a.Namespace
		}
		for _, res := range []string{"serviceaccounts", "roles", "rolebindings"} {
			if ns, ok := got[res]; !ok || ns != "prod" {
				t.Fatalf("expected create on %q in prod, got namespace %q (present=%v)", res, ns, ok)
			}
		}
	})
	t.Run("multi-namespace set needs create on roles/rolebindings in EACH namespace, SA in the first", func(t *testing.T) {
		attrs := buildCreateAttributes(false, "prod", []string{"prod", "staging"})
		// resource -> set of namespaces it was requested in
		ns := map[string]map[string]bool{}
		for _, a := range attrs {
			if ns[a.Resource] == nil {
				ns[a.Resource] = map[string]bool{}
			}
			ns[a.Resource][a.Namespace] = true
		}
		if !ns["serviceaccounts"]["prod"] || ns["serviceaccounts"]["staging"] {
			t.Fatalf("serviceaccounts create namespaces = %v, want only the first (prod)", ns["serviceaccounts"])
		}
		for _, res := range []string{"roles", "rolebindings"} {
			if !ns[res]["prod"] || !ns[res]["staging"] {
				t.Fatalf("expected create on %q in BOTH prod and staging, got %v", res, ns[res])
			}
		}
	})
	t.Run("cluster-wide set needs create on cluster roles/bindings cluster-wide", func(t *testing.T) {
		attrs := buildCreateAttributes(true, "default", nil)
		ns := map[string]string{}
		for _, a := range attrs {
			ns[a.Resource] = a.Namespace
		}
		if ns["serviceaccounts"] != "default" {
			t.Fatalf("serviceaccounts namespace = %q, want default", ns["serviceaccounts"])
		}
		for _, res := range []string{"clusterroles", "clusterrolebindings"} {
			if v, ok := ns[res]; !ok || v != "" {
				t.Fatalf("expected cluster-wide create on %q (empty namespace), got %q (present=%v)", res, v, ok)
			}
		}
	})
}

func TestParseNamespaceList(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    []string
		wantErr string
	}{
		{name: "single namespace", raw: "prod", want: []string{"prod"}},
		{name: "comma list preserves order", raw: "prod,staging,dev", want: []string{"prod", "staging", "dev"}},
		{name: "whitespace trimmed and duplicates dropped", raw: " prod , staging , prod ", want: []string{"prod", "staging"}},
		{name: "empty entry rejected", raw: "prod,,staging", wantErr: "empty namespace"},
		{name: "wildcard mixed into a list rejected", raw: "prod,*", wantErr: "wildcard"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseNamespaceList(tc.raw)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want one mentioning %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("parseNamespaceList(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestResolveNamespaceScope(t *testing.T) {
	ns := func(s string) *string { return &s }
	cases := []struct {
		name           string
		opts           mintOptions
		wantNamespaces []string
		wantAll        bool
		wantErr        string
	}{
		{
			name:           "explicit comma list yields the multi-namespace set",
			opts:           mintOptions{configFlags: &genericclioptions.ConfigFlags{Namespace: ns("prod,staging")}},
			wantNamespaces: []string{"prod", "staging"},
		},
		{
			name:    "the -A flag selects all-namespaces",
			opts:    mintOptions{allNamespaces: true, configFlags: &genericclioptions.ConfigFlags{Namespace: ns("")}},
			wantAll: true,
		},
		{
			name:    "the -n '*' sugar selects all-namespaces",
			opts:    mintOptions{configFlags: &genericclioptions.ConfigFlags{Namespace: ns("*")}},
			wantAll: true,
		},
		{
			name:    "all-namespaces cannot be combined with an explicit list",
			opts:    mintOptions{allNamespaces: true, configFlags: &genericclioptions.ConfigFlags{Namespace: ns("prod")}},
			wantErr: "all-namespaces",
		},
		{
			name:    "cluster-scoped cannot be combined with all-namespaces",
			opts:    mintOptions{clusterScoped: true, allNamespaces: true, configFlags: &genericclioptions.ConfigFlags{Namespace: ns("")}},
			wantErr: "cluster-scoped",
		},
		{
			name: "cluster-scoped yields neither a namespace list nor the wildcard",
			opts: mintOptions{clusterScoped: true, configFlags: &genericclioptions.ConfigFlags{Namespace: ns("")}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.opts.resolveNamespaceScope()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want one mentioning %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.all != tc.wantAll {
				t.Fatalf("all = %v, want %v", got.all, tc.wantAll)
			}
			if strings.Join(got.namespaces, ",") != strings.Join(tc.wantNamespaces, ",") {
				t.Fatalf("namespaces = %v, want %v", got.namespaces, tc.wantNamespaces)
			}
		})
	}
}

func TestScopeSummary(t *testing.T) {
	got := scopeSummary([]string{"get", "list", "watch"}, []string{"pods"})
	if got != "get,list,watch:pods" {
		t.Errorf("scopeSummary = %q, want %q", got, "get,list,watch:pods")
	}
}

func TestCreatedObjectNames(t *testing.T) {
	t.Run("namespaced names the namespaced kinds", func(t *testing.T) {
		got := createdObjectNames("tessera-x", false)
		want := []string{"serviceaccount/tessera-x", "role/tessera-x", "rolebinding/tessera-x"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("createdObjectNames = %v, want %v", got, want)
		}
	})
	t.Run("cluster-scoped names the cluster kinds", func(t *testing.T) {
		got := createdObjectNames("tessera-x", true)
		want := []string{"serviceaccount/tessera-x", "clusterrole/tessera-x", "clusterrolebinding/tessera-x"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("createdObjectNames = %v, want %v", got, want)
		}
	})
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
