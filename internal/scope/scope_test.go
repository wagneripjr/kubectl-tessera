package scope

import (
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// testMapper is a static RESTMapper covering the resources the FR-001 scenarios and
// the scope-resolution rules exercise: pods (namespaced core), nodes (cluster-scoped
// core), deployments (namespaced apps), and an ambiguous "events" (core +
// events.k8s.io).
func testMapper() meta.RESTMapper {
	m := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "", Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "events.k8s.io", Version: "v1"},
	})
	m.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"}, meta.RESTScopeRoot)
	m.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Event"}, meta.RESTScopeNamespace)
	m.Add(schema.GroupVersionKind{Group: "events.k8s.io", Version: "v1", Kind: "Event"}, meta.RESTScopeNamespace)
	return m
}

func TestResolveProducesScopedRules(t *testing.T) {
	cases := []struct {
		name          string
		req           Request
		wantResources []ResolvedResource
		wantRules     []PolicyRule
	}{
		{
			name:          "namespaced core resource",
			req:           Request{Verbs: []string{"get", "list", "watch"}, Resources: []string{"pods"}, Namespace: "prod"},
			wantResources: []ResolvedResource{{Resource: "pods", Group: "", Namespaced: true}},
			wantRules:     []PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "watch"}}},
		},
		{
			name:          "cluster-scoped core resource",
			req:           Request{Verbs: []string{"get", "list"}, Resources: []string{"nodes"}, ClusterScoped: true},
			wantResources: []ResolvedResource{{Resource: "nodes", Group: "", Namespaced: false}},
			wantRules:     []PolicyRule{{APIGroups: []string{""}, Resources: []string{"nodes"}, Verbs: []string{"get", "list"}}},
		},
		{
			name:          "non-core group resource",
			req:           Request{Verbs: []string{"get"}, Resources: []string{"deployments"}, Namespace: "prod"},
			wantResources: []ResolvedResource{{Resource: "deployments", Group: "apps", Namespaced: true}},
			wantRules:     []PolicyRule{{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get"}}},
		},
		{
			name:          "ambiguous resource disambiguated by api-group",
			req:           Request{Verbs: []string{"get"}, Resources: []string{"events"}, APIGroup: "events.k8s.io", Namespace: "prod"},
			wantResources: []ResolvedResource{{Resource: "events", Group: "events.k8s.io", Namespaced: true}},
			wantRules:     []PolicyRule{{APIGroups: []string{"events.k8s.io"}, Resources: []string{"events"}, Verbs: []string{"get"}}},
		},
		{
			name: "one rule per api group",
			req:  Request{Verbs: []string{"get"}, Resources: []string{"pods", "deployments"}, Namespace: "prod"},
			wantResources: []ResolvedResource{
				{Resource: "pods", Group: "", Namespaced: true},
				{Resource: "deployments", Group: "apps", Namespaced: true},
			},
			wantRules: []PolicyRule{
				{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
				{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get"}},
			},
		},
		{
			name:          "resource names are carried into the rule",
			req:           Request{Verbs: []string{"get"}, Resources: []string{"pods"}, ResourceNames: []string{"foo"}, Namespace: "prod"},
			wantResources: []ResolvedResource{{Resource: "pods", Group: "", Namespaced: true}},
			wantRules:     []PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}, ResourceNames: []string{"foo"}}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Resolve(tc.req, testMapper())
			if err != nil {
				t.Fatalf("Resolve returned error: %v", err)
			}
			if !reflect.DeepEqual(res.Resources, tc.wantResources) {
				t.Fatalf("resources = %+v, want %+v", res.Resources, tc.wantResources)
			}
			if !reflect.DeepEqual(res.Rules, tc.wantRules) {
				t.Fatalf("rules = %+v, want %+v", res.Rules, tc.wantRules)
			}
		})
	}
}

func TestResolveRejectsInconsistentScope(t *testing.T) {
	cases := []struct {
		name        string
		req         Request
		wantErrPart string
	}{
		{
			name:        "cluster-scoped resource without the flag",
			req:         Request{Verbs: []string{"get"}, Resources: []string{"nodes"}},
			wantErrPart: "cluster-scoped",
		},
		{
			name:        "namespaced resource with the cluster-scoped flag",
			req:         Request{Verbs: []string{"get"}, Resources: []string{"pods"}, ClusterScoped: true},
			wantErrPart: "namespaced",
		},
		{
			name:        "namespace passed with a cluster-scoped resource",
			req:         Request{Verbs: []string{"get"}, Resources: []string{"nodes"}, ClusterScoped: true, Namespace: "prod"},
			wantErrPart: "namespace",
		},
		{
			name:        "ambiguous resource without api-group",
			req:         Request{Verbs: []string{"get"}, Resources: []string{"events"}, Namespace: "prod"},
			wantErrPart: "api-group",
		},
		{
			name:        "no resources requested",
			req:         Request{Verbs: []string{"get"}, Namespace: "prod"},
			wantErrPart: "resource",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Resolve(tc.req, testMapper())
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErrPart) {
				t.Fatalf("error %q does not mention %q", err.Error(), tc.wantErrPart)
			}
		})
	}
}
