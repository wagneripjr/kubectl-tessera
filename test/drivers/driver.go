//go:build e2e

package drivers

import (
	"context"
	"time"
)

type MintMode int

const (
	ModeExec MintMode = iota

	ModePrintKubeconfig

	ModeDryRun
)

type MintRequest struct {
	Verbs         []string
	Resources     []string
	Namespaces    []string
	ResourceNames []string
	APIGroup      string
	TTL           time.Duration
	ClusterScoped bool

	AllNamespaces bool
	Mode          MintMode

	Output string

	AsIdentity string
}

type MintResult struct {
	ExitCode       int
	Stdout         string
	Stderr         string
	KubeconfigPath string
	SessionID      string
}

type TesseraDriver interface {
	Mint(ctx context.Context, req MintRequest) (MintResult, error)
	Gc(ctx context.Context) (MintResult, error)
	Ls(ctx context.Context) (MintResult, error)
	LsJSON(ctx context.Context) (MintResult, error)

	KillExecProcess(ctx context.Context, signal string) error

	SessionObjectsExist(ctx context.Context, sessionID string) (bool, error)
	SessionObjectsCount(ctx context.Context, sessionID string) (int, error)
	UnmanagedRBACExists(ctx context.Context, name string) (bool, error)

	MintedTokenCan(ctx context.Context, kubeconfigPath, verb, resource, group, namespace, name string) (bool, error)

	MintedTokenRequest(ctx context.Context, kubeconfigPath, resource, group, namespace string) (status int, err error)

	SeedLimitedIdentity(ctx context.Context, name string, verbs, resources, resourceNames []string) error
	SeedUnmanagedRBAC(ctx context.Context, name string) error
	SeedManagedSession(ctx context.Context, sessionID string, expiresAt time.Time) error
	KubeconfigFileExists(path string) (bool, error)

	EnsureNamespace(ctx context.Context, namespace string) error
	DeleteNamespace(ctx context.Context, namespace string) error
	DeleteSessionByLabel(ctx context.Context, sessionID string) error
	Close()
}
