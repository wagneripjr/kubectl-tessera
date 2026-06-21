//go:build e2e

// Package dsl is the assertion layer of the acceptance suite. It translates
// business concepts into protocol-driver calls and holds ALL assertions (ATDD Gate
// G5). Methods return error: under godog a non-nil error fails the step. The DSL
// knows WHAT to verify, never HOW to talk to the system.
package dsl

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/wagneripjr/kubectl-tessera/test/drivers"
)

const (
	limitedIdentity        = "tessera-limited"
	partialCreatorIdentity = "tessera-partial-creator"
)

// SessionDSL carries per-scenario state across Given/When/Then steps.
type SessionDSL struct {
	driver *drivers.KindDriver
	req    drivers.MintRequest
	last   drivers.MintResult
}

// New returns a fresh DSL bound to the driver for one scenario.
func New(driver *drivers.KindDriver) *SessionDSL {
	return &SessionDSL{driver: driver}
}

// --- Given ---

// GivenOperatorRequests sets up a namespaced request for the given verbs/resource.
func (s *SessionDSL) GivenOperatorRequests(verbs, resource string) {
	s.req = drivers.MintRequest{
		Verbs:     splitCSV(verbs),
		Resources: []string{resource},
		TTL:       15 * time.Minute,
		Mode:      drivers.ModePrintKubeconfig,
	}
}

// GivenOperatorRequestsClusterScoped sets up a cluster-scoped request.
func (s *SessionDSL) GivenOperatorRequestsClusterScoped(verbs, resource string) {
	s.req = drivers.MintRequest{
		Verbs:         splitCSV(verbs),
		Resources:     []string{resource},
		ClusterScoped: true,
		TTL:           15 * time.Minute,
		Mode:          drivers.ModePrintKubeconfig,
	}
}

// GivenOperatorRequestsNamed sets up a name-narrowed request.
func (s *SessionDSL) GivenOperatorRequestsNamed(verb, resource, name string) {
	s.req = drivers.MintRequest{
		Verbs:         []string{verb},
		Resources:     []string{resource},
		ResourceNames: []string{name},
		TTL:           15 * time.Minute,
		Mode:          drivers.ModePrintKubeconfig,
	}
}

// GivenLimitedOperator seeds a deliberately under-privileged identity that may only
// read the given resource, and arranges for subsequent mints to run AS it.
func (s *SessionDSL) GivenLimitedOperator(ctx context.Context, resource string) error {
	if err := s.driver.SeedLimitedIdentity(ctx, limitedIdentity,
		[]string{"get", "list", "watch"}, []string{resource}, nil); err != nil {
		return fmt.Errorf("seeding limited identity: %w", err)
	}
	s.req.AsIdentity = limitedIdentity
	return nil
}

// GivenSubMinimumSession mints a read-only session requesting a lifetime below the
// cluster's minimum token TTL (the kube-apiserver hardcoded 10-minute floor). The
// requested 5m is well under that floor, so the SUT must floor it up to the minimum
// for the mint to succeed at all. The window stays comfortably large so nothing
// lapses mid-assertion; the scenario performs no waiting.
func (s *SessionDSL) GivenSubMinimumSession(ctx context.Context, resource string) error {
	s.req = drivers.MintRequest{
		Verbs:     []string{"get", "list", "watch"},
		Resources: []string{resource},
		TTL:       5 * time.Minute,
		Mode:      drivers.ModePrintKubeconfig,
	}
	return s.mint(ctx)
}

// GivenInteractiveSession mints an interactive (--exec) read-only session.
func (s *SessionDSL) GivenInteractiveSession(ctx context.Context, resource string) error {
	s.req = drivers.MintRequest{
		Verbs:     []string{"get", "list", "watch"},
		Resources: []string{resource},
		TTL:       15 * time.Minute,
		Mode:      drivers.ModeExec,
	}
	return s.mint(ctx)
}

// GivenShortLivedInteractiveSession mints an interactive (--exec) read-only session
// with a lifetime short enough to elapse within the scenario's wait, and leaves the
// process RUNNING in the background (on a real blocking shell) so a later SIGKILL
// bypasses the exit trap and orphans the set. The token is floored to the cluster
// minimum, but the expires-at annotation reflects the requested 1s — and the sweep
// reads the annotation, so the orphaned objects become reclaimable after the wait.
// (FR-011 crash recovery.)
func (s *SessionDSL) GivenShortLivedInteractiveSession(ctx context.Context, resource string) error {
	s.req = drivers.MintRequest{
		Verbs:     []string{"get", "list", "watch"},
		Resources: []string{resource},
		TTL:       1 * time.Second,
		Mode:      drivers.ModeExec,
	}
	res, err := s.driver.MintExecBackground(ctx, s.req)
	if err != nil {
		return fmt.Errorf("starting background exec session: %w", err)
	}
	s.last = res
	return nil
}

// GivenExpiredManagedSession seeds a managed session whose expiry is in the past.
func (s *SessionDSL) GivenExpiredManagedSession(ctx context.Context, sessionID string) error {
	return s.driver.SeedManagedSession(ctx, sessionID, time.Now().Add(-time.Hour))
}

// GivenUnexpiredManagedSession seeds a managed session whose expiry is in the future.
func (s *SessionDSL) GivenUnexpiredManagedSession(ctx context.Context, sessionID string) error {
	return s.driver.SeedManagedSession(ctx, sessionID, time.Now().Add(time.Hour))
}

// GivenUnmanagedRoleBinding seeds a RoleBinding not managed by tessera.
func (s *SessionDSL) GivenUnmanagedRoleBinding(ctx context.Context, name string) error {
	return s.driver.SeedUnmanagedRBAC(ctx, name)
}

// --- When ---

// WhenOperatorMints mints the session described by the accumulated request.
func (s *SessionDSL) WhenOperatorMints(ctx context.Context) error { return s.mint(ctx) }

// WhenLimitedOperatorRequests mints AS the limited identity for an over-ask probe.
func (s *SessionDSL) WhenLimitedOperatorRequests(ctx context.Context, verb, resource string) error {
	s.req.Verbs = []string{verb}
	s.req.Resources = []string{resource}
	s.req.AsIdentity = limitedIdentity
	s.req.Mode = drivers.ModePrintKubeconfig
	return s.mint(ctx)
}

// WhenOperatorMintsPrintKubeconfig mints in print-kubeconfig mode for non-interactive use.
func (s *SessionDSL) WhenOperatorMintsPrintKubeconfig(ctx context.Context) error {
	s.req.Mode = drivers.ModePrintKubeconfig
	return s.mint(ctx)
}

// WhenOperatorLeavesSession ends the interactive session.
func (s *SessionDSL) WhenOperatorLeavesSession(_ context.Context) error {
	// The subshell exits on its own (SHELL=/usr/bin/true); cleanup is the SUT's job.
	return nil
}

// WhenSessionTerminatedAbruptly force-kills the interactive session process.
func (s *SessionDSL) WhenSessionTerminatedAbruptly(ctx context.Context) error {
	return s.driver.KillExecProcess(ctx, "SIGKILL")
}

// WhenSessionLifetimeElapses waits past the session's lifetime.
func (s *SessionDSL) WhenSessionLifetimeElapses(_ context.Context) error {
	time.Sleep(3 * time.Second)
	return nil
}

// WhenGarbageCollectionRuns runs the gc sweep.
func (s *SessionDSL) WhenGarbageCollectionRuns(ctx context.Context) error {
	res, err := s.driver.Gc(ctx)
	if err != nil {
		return err
	}
	// gc is a separate invocation that carries no session identity. Preserve the minted
	// session's id/kubeconfig recorded by the Given so the crash-recovery assertion
	// (ThenSessionObjectsGone) can still prove a session actually existed before the sweep.
	res.SessionID = s.last.SessionID
	res.KubeconfigPath = s.last.KubeconfigPath
	s.last = res
	return nil
}

// GivenCreationWillFail arranges a real mid-creation failure (FR-005): it seeds an
// operator that may read pods and create/delete ServiceAccounts and Roles but may NOT
// create RoleBindings, and points the next mint AT it. The binary then creates the SA
// and Role and fails at the binding (403), so reverse-order rollback must run.
func (s *SessionDSL) GivenCreationWillFail(ctx context.Context) error {
	if err := s.driver.SeedPartialCreatorIdentity(ctx, partialCreatorIdentity); err != nil {
		return fmt.Errorf("seeding partial-creator identity: %w", err)
	}
	s.req = drivers.MintRequest{
		Verbs:      []string{"get", "list", "watch"},
		Resources:  []string{"pods"},
		TTL:        15 * time.Minute,
		Mode:       drivers.ModePrintKubeconfig,
		AsIdentity: partialCreatorIdentity,
	}
	return nil
}

// --- Then (assertions) ---

// ThenMintedCredentialCan asserts the minted token's effective authorization on a
// (verb, resource[, name]) — the load-bearing check via auth-can-i semantics.
func (s *SessionDSL) ThenMintedCredentialCan(ctx context.Context, outcome, verb, resource, name string) error {
	if s.last.KubeconfigPath == "" {
		return fmt.Errorf("mint produced no kubeconfig (exit %d): %s", s.last.ExitCode, strings.TrimSpace(s.last.Stderr))
	}
	ns := s.driver.Namespace()
	if s.req.ClusterScoped {
		ns = ""
	}
	allowed, err := s.driver.MintedTokenCan(ctx, s.last.KubeconfigPath, verb, resource, "", ns, name)
	if err != nil {
		return fmt.Errorf("checking minted authz: %w", err)
	}
	want := outcome == "is allowed"
	if allowed != want {
		return fmt.Errorf("expected %q to %q %q (name=%q) but allowed=%v", verb, outcome, resource, name, allowed)
	}
	return nil
}

// ThenMintRefused asserts the mint was rejected.
func (s *SessionDSL) ThenMintRefused() error {
	if s.last.ExitCode == 0 {
		return fmt.Errorf("expected the mint to be refused (non-zero exit), got exit 0")
	}
	return nil
}

// ThenAllowedDeniedReported asserts the allowed/denied scope was reported.
func (s *SessionDSL) ThenAllowedDeniedReported() error {
	out := s.last.Stdout + s.last.Stderr
	if !regexp.MustCompile(`(?i)(denied|allowed)`).MatchString(out) {
		return fmt.Errorf("expected an allowed/denied report, got: %s", strings.TrimSpace(out))
	}
	return nil
}

// ThenNoManagedObjectsCreated asserts the refused attempt created nothing.
func (s *SessionDSL) ThenNoManagedObjectsCreated(ctx context.Context) error {
	n, err := s.driver.CountManaged(ctx)
	if err != nil {
		return err
	}
	if n != 0 {
		return fmt.Errorf("expected no managed objects, found %d", n)
	}
	return nil
}

// ThenNoManagedObjectsRemain asserts that a mint which failed partway through left no
// managed objects behind (FR-005). It first requires the mint to have failed — a
// successful mint would leave a full set, so this guards against the fault not firing —
// then polls until the count settles to zero, because the SUT deletes with foreground
// propagation and the Delete call returns before finalizers clear.
func (s *SessionDSL) ThenNoManagedObjectsRemain(ctx context.Context) error {
	if s.last.ExitCode == 0 {
		return fmt.Errorf("expected the mint to fail mid-creation, got exit 0: %s", strings.TrimSpace(s.last.Stderr))
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		n, err := s.driver.CountManaged(ctx)
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("expected no managed objects to remain after rollback, found %d", n)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// ThenMintedCredentialWorks asserts the minted token is currently accepted.
func (s *SessionDSL) ThenMintedCredentialWorks(ctx context.Context, resource string) error {
	if s.last.KubeconfigPath == "" {
		return fmt.Errorf("mint produced no kubeconfig (exit %d): %s", s.last.ExitCode, strings.TrimSpace(s.last.Stderr))
	}
	status, err := s.driver.MintedTokenRequest(ctx, s.last.KubeconfigPath, resource, "", s.driver.Namespace())
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("expected the credential to work (200), got %d", status)
	}
	return nil
}

// ThenWarnedTTLFloored asserts the SUT warned that the requested lifetime was floored
// up to the cluster minimum. The diagnostic stream is the contract surface (NFR-008);
// the substring is the stable part of the warning the operator reads.
func (s *SessionDSL) ThenWarnedTTLFloored() error {
	if !strings.Contains(s.last.Stderr, "floored to cluster minimum") {
		return fmt.Errorf("expected a floor warning on the diagnostic output, got: %s", strings.TrimSpace(s.last.Stderr))
	}
	return nil
}

// ThenSessionObjectsGone asserts the session's managed objects are gone. It first
// requires that a session was actually minted (a session-id was recorded) — without
// this guard the assertion is vacuously true when the mint creates nothing (e.g. an
// unimplemented --exec), which would let a do-nothing SUT pass. It then polls, because
// the SUT deletes with foreground propagation: the objects can still be finalizing when
// the process exits, so a single read races the cluster. A do-nothing cleanup never
// converges, so the teeth are preserved.
func (s *SessionDSL) ThenSessionObjectsGone(ctx context.Context) error {
	if s.last.SessionID == "" {
		return fmt.Errorf("expected a session to have been minted, but no session-id was recorded (exit %d): %s",
			s.last.ExitCode, strings.TrimSpace(s.last.Stderr))
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		n, err := s.driver.CountManaged(ctx)
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("expected the session's managed objects to be gone, found %d", n)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// ThenKubeconfigFileRemoved asserts the session kubeconfig file is gone. It first
// requires that a kubeconfig path was recorded — otherwise the check is vacuously
// true (KubeconfigFileExists("") is always false), which would let an --exec that
// never wrote a kubeconfig pass.
func (s *SessionDSL) ThenKubeconfigFileRemoved() error {
	if s.last.KubeconfigPath == "" {
		return fmt.Errorf("expected a session kubeconfig to have been created, but no path was recorded (exit %d): %s",
			s.last.ExitCode, strings.TrimSpace(s.last.Stderr))
	}
	exists, err := s.driver.KubeconfigFileExists(s.last.KubeconfigPath)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("expected the session kubeconfig file to be removed: %s", s.last.KubeconfigPath)
	}
	return nil
}

// ThenManagedSessionRemoved asserts a seeded managed session is gone.
func (s *SessionDSL) ThenManagedSessionRemoved(ctx context.Context, sessionID string) error {
	exists, err := s.driver.SessionObjectsExist(ctx, sessionID)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("expected managed session %q to be removed", sessionID)
	}
	return nil
}

// ThenManagedSessionRemains asserts a seeded managed session is still present.
func (s *SessionDSL) ThenManagedSessionRemains(ctx context.Context, sessionID string) error {
	exists, err := s.driver.SessionObjectsExist(ctx, sessionID)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("expected managed session %q to remain", sessionID)
	}
	return nil
}

// ThenUnmanagedRoleBindingRemains asserts an unmanaged RoleBinding is untouched.
func (s *SessionDSL) ThenUnmanagedRoleBindingRemains(ctx context.Context, name string) error {
	exists, err := s.driver.UnmanagedRBACExists(ctx, name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("expected unmanaged role binding %q to remain", name)
	}
	return nil
}

// ThenOnlyPathOnStdout asserts stdout is exactly one filesystem path.
func (s *SessionDSL) ThenOnlyPathOnStdout() error {
	out := strings.TrimRight(s.last.Stdout, "\n")
	if out == "" || strings.Contains(out, "\n") {
		return fmt.Errorf("expected exactly one path on stdout, got: %q", s.last.Stdout)
	}
	if !strings.HasPrefix(out, "/") {
		return fmt.Errorf("expected an absolute path on stdout, got: %q", out)
	}
	return nil
}

// ThenKubeconfigGrantsReadAccess asserts the produced kubeconfig grants the read.
func (s *SessionDSL) ThenKubeconfigGrantsReadAccess(ctx context.Context, resource string) error {
	return s.ThenMintedCredentialCan(ctx, "is allowed", "get", resource, "")
}

// ThenAuditOnDiagnosticOutput asserts the audit details went to stderr, not stdout.
func (s *SessionDSL) ThenAuditOnDiagnosticOutput() error {
	if !strings.Contains(s.last.Stderr, "session") {
		return fmt.Errorf("expected audit details on the diagnostic output (stderr), got: %s", strings.TrimSpace(s.last.Stderr))
	}
	return nil
}

// --- internal ---

func (s *SessionDSL) mint(ctx context.Context) error {
	res, err := s.driver.Mint(ctx, s.req)
	if err != nil {
		return fmt.Errorf("invoking mint: %w", err)
	}
	s.last = res
	return nil
}

func splitCSV(s string) []string { return strings.Split(s, ",") }
