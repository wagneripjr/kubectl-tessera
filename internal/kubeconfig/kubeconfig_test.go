package kubeconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestBuildRendersSingleContext(t *testing.T) {
	cases := []struct {
		name   string
		params func(t *testing.T) Params
		check  func(t *testing.T, cluster *clientcmdapi.Cluster, ctx *clientcmdapi.Context, auth *clientcmdapi.AuthInfo)
	}{
		{
			name: "embeds server, inline CA and token",
			params: func(t *testing.T) Params {
				return Params{
					RESTConfig: &rest.Config{Host: "https://api.example:6443", TLSClientConfig: rest.TLSClientConfig{CAData: []byte("CADATA")}},
					Token:      "minted-token",
					Namespace:  "prod",
					SessionID:  "1a2b3c4d",
				}
			},
			check: func(t *testing.T, cluster *clientcmdapi.Cluster, ctx *clientcmdapi.Context, auth *clientcmdapi.AuthInfo) {
				if cluster.Server != "https://api.example:6443" {
					t.Fatalf("server = %q", cluster.Server)
				}
				if string(cluster.CertificateAuthorityData) != "CADATA" {
					t.Fatalf("CA data = %q, want CADATA", cluster.CertificateAuthorityData)
				}
				if auth.Token != "minted-token" {
					t.Fatalf("token = %q, want minted-token", auth.Token)
				}
				if ctx.Namespace != "prod" {
					t.Fatalf("namespace = %q, want prod", ctx.Namespace)
				}
			},
		},
		{
			name: "reads CA from file when no inline data",
			params: func(t *testing.T) Params {
				caFile := filepath.Join(t.TempDir(), "ca.crt")
				if err := os.WriteFile(caFile, []byte("FILE-CA"), 0o600); err != nil {
					t.Fatal(err)
				}
				return Params{
					RESTConfig: &rest.Config{Host: "https://api:6443", TLSClientConfig: rest.TLSClientConfig{CAFile: caFile}},
					Token:      "tok",
					SessionID:  "id",
				}
			},
			check: func(t *testing.T, cluster *clientcmdapi.Cluster, _ *clientcmdapi.Context, _ *clientcmdapi.AuthInfo) {
				if string(cluster.CertificateAuthorityData) != "FILE-CA" {
					t.Fatalf("CA data = %q, want FILE-CA", cluster.CertificateAuthorityData)
				}
			},
		},
		{
			name: "propagates insecure-skip-tls-verify",
			params: func(t *testing.T) Params {
				return Params{
					RESTConfig: &rest.Config{Host: "https://api:6443", TLSClientConfig: rest.TLSClientConfig{Insecure: true}},
					Token:      "tok",
					SessionID:  "id",
				}
			},
			check: func(t *testing.T, cluster *clientcmdapi.Cluster, _ *clientcmdapi.Context, _ *clientcmdapi.AuthInfo) {
				if !cluster.InsecureSkipTLSVerify {
					t.Fatal("InsecureSkipTLSVerify = false, want true")
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Build(tc.params(t))
			if len(cfg.Clusters) != 1 || len(cfg.AuthInfos) != 1 || len(cfg.Contexts) != 1 {
				t.Fatalf("config should have exactly one cluster/authinfo/context: %+v", cfg)
			}
			ctx := cfg.Contexts[cfg.CurrentContext]
			tc.check(t, cfg.Clusters[ctx.Cluster], ctx, cfg.AuthInfos[ctx.AuthInfo])
		})
	}
}

func TestPathIsAbsoluteAndSessionScoped(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/501")
	p := Path("1a2b3c4d")
	if !filepath.IsAbs(p) {
		t.Fatalf("path %q is not absolute", p)
	}
	if !strings.Contains(p, "1a2b3c4d") || !strings.HasSuffix(p, ".kubeconfig") {
		t.Fatalf("path %q should be session-scoped and end in .kubeconfig", p)
	}
}

func TestWriteCreatesPrivateFileInPrivateDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	cfg := Build(Params{
		RESTConfig: &rest.Config{Host: "https://api:6443", TLSClientConfig: rest.TLSClientConfig{CAData: []byte("CA")}},
		Token:      "tok",
		Namespace:  "prod",
		SessionID:  "1a2b3c4d",
	})

	path, err := Write(cfg, "1a2b3c4d")
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("kubeconfig not written: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("kubeconfig mode = %v, want 0600 (NFR-001)", fi.Mode().Perm())
	}
	di, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if di.Mode().Perm() != 0o700 {
		t.Fatalf("parent dir mode = %v, want 0700", di.Mode().Perm())
	}
	if _, err := clientcmd.BuildConfigFromFlags("", path); err != nil {
		t.Fatalf("written kubeconfig is not loadable: %v", err)
	}
}
