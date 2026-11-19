package kubeconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const defaultName = "tessera"

type Params struct {
	RESTConfig *rest.Config
	Token      string
	Namespace  string
	SessionID  string
	Name       string
}

func Name(sourceCluster string) string {
	if sourceCluster == "" {
		return defaultName
	}
	return defaultName + "@" + sourceCluster
}

func Build(p Params) *clientcmdapi.Config {
	name := p.Name
	if name == "" {
		name = defaultName
	}
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters[name] = &clientcmdapi.Cluster{
		Server:                   p.RESTConfig.Host,
		CertificateAuthorityData: caData(p.RESTConfig),
		InsecureSkipTLSVerify:    p.RESTConfig.Insecure,
	}
	cfg.AuthInfos[name] = &clientcmdapi.AuthInfo{Token: p.Token}
	cfg.Contexts[name] = &clientcmdapi.Context{
		Cluster:   name,
		AuthInfo:  name,
		Namespace: p.Namespace,
	}
	cfg.CurrentContext = name
	return cfg
}

func Path(sessionID string) string {
	return filepath.Join(runtimeDir(), sessionID+".kubeconfig")
}

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
