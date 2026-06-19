//go:build e2e

// Package drivers holds the ATDD protocol driver for the acceptance suite. The
// driver is the only bridge between the executable specs and the system under test:
// a composite of a process adapter (spawns the compiled kubectl-tessera binary) and
// a cluster adapter (client-go, verifies real cluster state and the effective authz
// of the MINTED token). It contains ZERO assertions — assertions live in test/dsl
// (ATDD Gate G5). See docs/design/protocol-drivers.md.
package drivers

import (
	"context"
	"time"
)

// MintMode selects how a minted session is delivered.
type MintMode int

const (
	// ModeExec spawns an interactive subshell and cleans up on exit (default).
	ModeExec MintMode = iota
	// ModePrintKubeconfig prints the kubeconfig path and leaves objects for gc.
	ModePrintKubeconfig
	// ModeDryRun runs pre-flight and prints intended objects, creating nothing.
	ModeDryRun
)

// MintRequest is the operator's request in domain terms.
type MintRequest struct {
	Verbs         []string
	Resources     []string
	Namespaces    []string
	ResourceNames []string
	APIGroup      string
	TTL           time.Duration
	ClusterScoped bool
	Mode          MintMode
	// AsIdentity selects which identity tessera runs as. "" = admin context;
	// otherwise the name of a seeded limited identity (see SeedLimitedIdentity).
	AsIdentity string
}

// MintResult is everything observable from one invocation of the binary.
type MintResult struct {
	ExitCode       int
	Stdout         string
	Stderr         string
	KubeconfigPath string // parsed from stdout in ModePrintKubeconfig
	SessionID      string // parsed from the stderr audit line or -o json
}

// TesseraDriver drives the SUT through its real external protocols. No assertions.
type TesseraDriver interface {
	// --- process adapter: the real CLI ---
	Mint(ctx context.Context, req MintRequest) (MintResult, error)
	Gc(ctx context.Context) (MintResult, error)
	Ls(ctx context.Context) (MintResult, error)
	// KillExecProcess force-kills the most recent --exec child (crash recovery #5).
	KillExecProcess(ctx context.Context, signal string) error

	// --- cluster adapter: real state via client-go (admin context) ---
	SessionObjectsExist(ctx context.Context, sessionID string) (bool, error)
	SessionObjectsCount(ctx context.Context, sessionID string) (int, error)
	UnmanagedRBACExists(ctx context.Context, name string) (bool, error)

	// --- effective authz of the MINTED token (the load-bearing checks) ---
	// MintedTokenCan runs an SSAR with the minted token (auth can-i semantics).
	MintedTokenCan(ctx context.Context, kubeconfigPath, verb, resource, group, namespace, name string) (bool, error)
	// MintedTokenRequest issues a real request with the minted token, returning the
	// HTTP status (200 on success, 401/403 on rejection — used for TTL expiry #3).
	MintedTokenRequest(ctx context.Context, kubeconfigPath, resource, group, namespace string) (status int, err error)

	// --- harness helpers (admin context) ---
	SeedLimitedIdentity(ctx context.Context, name string, verbs, resources, resourceNames []string) error
	SeedUnmanagedRBAC(ctx context.Context, name string) error
	SeedManagedSession(ctx context.Context, sessionID string, expiresAt time.Time) error
	KubeconfigFileExists(path string) (bool, error)

	// --- lifecycle ---
	EnsureNamespace(ctx context.Context, namespace string) error
	DeleteNamespace(ctx context.Context, namespace string) error
	DeleteSessionByLabel(ctx context.Context, sessionID string) error
	Close()
}
