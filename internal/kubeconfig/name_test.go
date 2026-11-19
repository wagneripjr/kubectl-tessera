package kubeconfig

import (
	"testing"

	"k8s.io/client-go/rest"
)

func TestNameDerivesFromSourceCluster(t *testing.T) {
	cases := map[string]string{
		"kind-kind": "tessera@kind-kind",
		"":          "tessera",
	}
	for source, want := range cases {
		if got := Name(source); got != want {
			t.Fatalf("Name(%q) = %q, want %q", source, got, want)
		}
	}
}

func TestBuildNamesClusterContextAndUser(t *testing.T) {
	cases := map[string]string{
		"tessera@prod": "tessera@prod",
		"":             "tessera",
	}
	for name, want := range cases {
		cfg := Build(Params{RESTConfig: &rest.Config{Host: "https://api:6443"}, Token: "tok", SessionID: "id", Name: name})
		ctx, ok := cfg.Contexts[want]
		if cfg.CurrentContext != want || !ok {
			t.Fatalf("Name %q: current context = %q, want %q", name, cfg.CurrentContext, want)
		}
		if ctx.Cluster != want || ctx.AuthInfo != want {
			t.Fatalf("Name %q: context points at cluster %q user %q, want both %q", name, ctx.Cluster, ctx.AuthInfo, want)
		}
		if _, ok := cfg.Clusters[want]; !ok {
			t.Fatalf("Name %q: no cluster entry %q", name, want)
		}
	}
}
