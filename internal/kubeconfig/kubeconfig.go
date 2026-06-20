// Package kubeconfig builds an isolated, 0600 throwaway kubeconfig containing only
// the minted token, the source cluster's server+CA, and a context bound to the
// target namespace. It never touches the user's ~/.kube/config. See FR-007, NFR-001.
package kubeconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// contextName is the single cluster/authinfo/context name used in the throwaway
// kubeconfig. A fixed name keeps the file self-contained and predictable.
const contextName = "tessera"

// Params carries everything needed to render the throwaway kubeconfig.
type Params struct {
	RESTConfig *rest.Config // source config (server + CA) — never its credentials
	Token      string
	Namespace  string // context namespace ("" allowed for cluster-scoped sessions)
	SessionID  string
}

// Build renders a single-context kubeconfig pointing at the source cluster but
// authenticating with the minted token. It is pure (no filesystem access).
func Build(p Params) *clientcmdapi.Config {
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters[contextName] = &clientcmdapi.Cluster{
		Server:                   p.RESTConfig.Host,
		CertificateAuthorityData: caData(p.RESTConfig),
		InsecureSkipTLSVerify:    p.RESTConfig.Insecure,
	}
	cfg.AuthInfos[contextName] = &clientcmdapi.AuthInfo{Token: p.Token}
	cfg.Contexts[contextName] = &clientcmdapi.Context{
		Cluster:   contextName,
		AuthInfo:  contextName,
		Namespace: p.Namespace,
	}
	cfg.CurrentContext = contextName
	return cfg
}

// Path returns the absolute throwaway-kubeconfig path for a session, under
// ${XDG_RUNTIME_DIR:-/tmp}/tessera.
func Path(sessionID string) string {
	return filepath.Join(runtimeDir(), sessionID+".kubeconfig")
}

// Write persists cfg to the session path with 0600 permissions inside a 0700
// directory, and returns the path. The caller prints this path (and only this path)
// to stdout in --print-kubeconfig mode.
func Write(cfg *clientcmdapi.Config, sessionID string) (string, error) {
	dir := runtimeDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating runtime dir %s: %w", dir, err)
	}
	path := filepath.Join(dir, sessionID+".kubeconfig")
	if err := clientcmd.WriteToFile(*cfg, path); err != nil {
		return "", fmt.Errorf("writing kubeconfig %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", fmt.Errorf("securing kubeconfig %s: %w", path, err)
	}
	return path, nil
}

// caData returns inline CA bytes, falling back to reading CAFile when no inline
// data is present. A read failure yields nil — the resulting config simply lacks a
// CA (e.g. an insecure cluster), which Build still renders.
func caData(c *rest.Config) []byte {
	if len(c.CAData) > 0 {
		return c.CAData
	}
	if c.CAFile != "" {
		if b, err := os.ReadFile(c.CAFile); err == nil {
			return b
		}
	}
	return nil
}

func runtimeDir() string {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = "/tmp"
	}
	return filepath.Join(base, "tessera")
}
