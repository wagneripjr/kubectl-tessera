//go:build e2e

// Package dsl is the assertion layer of the acceptance suite. It translates
// business concepts into protocol-driver calls and holds ALL assertions (ATDD Gate
// G5). Methods return error: under godog a non-nil error fails the step. The DSL
// knows WHAT to verify, never HOW to talk to the system.
package dsl

import (
	"context"
	"encoding/json"
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
	driver   *drivers.KindDriver
	req      drivers.MintRequest
	last     drivers.MintResult
	mintedID string // session-id of a session minted earlier in the scenario, preserved
	// across a subsequent `ls` call (which overwrites s.last with the ls output).

	// Multi-namespace state (FR-017): the namespaces the session was asked to span,
	// and one that was deliberately left out, so reachability can be proven each way.
	requestedNamespaces  []string
	unrequestedNamespace string
	// baselineManaged is the cluster-wide managed-object count captured just before an
	// expected-refusal mint, so the "created nothing anywhere" check (FR-018) has teeth
	// even when earlier scenarios left managed objects behind.
	baselineManaged int
}

// inventoryEntry is the black-box view of one `ls -o json` record. Defined here, not
// imported from internal/session, so the acceptance suite stays black-box (ADR-008,
// Gate G4): a change to the JSON contract SHOULD break these tests.
type inventoryEntry struct {
	SessionID string `json:"sessionID"`
	Owner     string `json:"owner"`
	ExpiresAt string `json:"expiresAt"`
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

// GivenOperatorRequestsAcrossTwoNamespaces sets up an explicit multi-namespace request
// (FR-017). It uses the scenario namespace plus a freshly-created sibling as the two
// requested namespaces, and creates a third, unrequested namespace so the negative
// reachability probe lands on a namespace that really exists.
func (s *SessionDSL) GivenOperatorRequestsAcrossTwoNamespaces(ctx context.Context, verbs, resource string) error {
	nsA := s.driver.Namespace()
	nsB := nsA + "-b"
	nsC := nsA + "-c"
	if err := s.driver.CreateTrackedNamespace(ctx, nsB); err != nil {
		return fmt.Errorf("creating second requested namespace: %w", err)
	}
	if err := s.driver.CreateTrackedNamespace(ctx, nsC); err != nil {
		return fmt.Errorf("creating unrequested namespace: %w", err)
	}
	s.requestedNamespaces = []string{nsA, nsB}
	s.unrequestedNamespace = nsC
	s.req = drivers.MintRequest{
		Verbs:      splitCSV(verbs),
		Resources:  []string{resource},
		Namespaces: []string{nsA, nsB},
		TTL:        15 * time.Minute,
		Mode:       drivers.ModePrintKubeconfig,
	}
	return nil
}

// GivenOperatorRequestsAllNamespaces sets up an all-namespaces (wildcard) request (FR-018).
func (s *SessionDSL) GivenOperatorRequestsAllNamespaces(verbs, resource string) {
	s.req = drivers.MintRequest{
		Verbs:         splitCSV(verbs),
		Resources:     []string{resource},
		AllNamespaces: true,
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

// WhenLimitedOperatorRequestsAllNamespaces mints AS the limited identity asking for an
// all-namespaces grant (FR-018). It captures the cluster-wide managed-object count first
// so the "created nothing anywhere" check can prove the refusal leaked nothing.
func (s *SessionDSL) WhenLimitedOperatorRequestsAllNamespaces(ctx context.Context, verb, resource string) error {
	n, err := s.driver.CountAllManaged(ctx)
	if err != nil {
		return fmt.Errorf("counting managed objects before the attempt: %w", err)
	}
	s.baselineManaged = n
	s.req.Verbs = []string{verb}
	s.req.Resources = []string{resource}
	s.req.AllNamespaces = true
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

// ThenCredentialReachesEachRequestedNamespace asserts the multi-namespace credential is
// authorized for the given verb/resource in every namespace the session requested (FR-017).
func (s *SessionDSL) ThenCredentialReachesEachRequestedNamespace(ctx context.Context, verb, resource string) error {
	if s.last.KubeconfigPath == "" {
		return fmt.Errorf("mint produced no kubeconfig (exit %d): %s", s.last.ExitCode, strings.TrimSpace(s.last.Stderr))
	}
	if len(s.requestedNamespaces) == 0 {
		return fmt.Errorf("no requested namespaces were recorded for this session")
	}
	for _, ns := range s.requestedNamespaces {
		allowed, err := s.driver.MintedTokenCan(ctx, s.last.KubeconfigPath, verb, resource, "", ns, "")
		if err != nil {
			return fmt.Errorf("checking minted authz in %q: %w", ns, err)
		}
		if !allowed {
			return fmt.Errorf("expected the credential to %q %q in requested namespace %q, but it was denied", verb, resource, ns)
		}
	}
	return nil
}

// ThenCredentialDeniedInUnrequestedNamespace asserts the multi-namespace credential is NOT
// authorized in a namespace the session did not request (FR-017) — proving the grant is the
// requested set, not cluster-wide.
func (s *SessionDSL) ThenCredentialDeniedInUnrequestedNamespace(ctx context.Context, verb, resource string) error {
	if s.unrequestedNamespace == "" {
		return fmt.Errorf("no unrequested namespace was recorded for this session")
	}
	allowed, err := s.driver.MintedTokenCan(ctx, s.last.KubeconfigPath, verb, resource, "", s.unrequestedNamespace, "")
	if err != nil {
		return fmt.Errorf("checking minted authz in %q: %w", s.unrequestedNamespace, err)
	}
	if allowed {
		return fmt.Errorf("expected the credential to be denied %q %q in unrequested namespace %q, but it was allowed", verb, resource, s.unrequestedNamespace)
	}
	return nil
}

// ThenCredentialReachesANewNamespace creates a namespace AFTER the session was minted and
// asserts the all-namespaces credential is authorized in it (FR-018) — the property an
// enumerate-current-namespaces design could never satisfy.
func (s *SessionDSL) ThenCredentialReachesANewNamespace(ctx context.Context, verb, resource string) error {
	if s.last.KubeconfigPath == "" {
		return fmt.Errorf("mint produced no kubeconfig (exit %d): %s", s.last.ExitCode, strings.TrimSpace(s.last.Stderr))
	}
	nsNew := s.driver.Namespace() + "-new"
	if err := s.driver.CreateTrackedNamespace(ctx, nsNew); err != nil {
		return fmt.Errorf("creating the after-the-fact namespace: %w", err)
	}
	allowed, err := s.driver.MintedTokenCan(ctx, s.last.KubeconfigPath, verb, resource, "", nsNew, "")
	if err != nil {
		return fmt.Errorf("checking minted authz in new namespace %q: %w", nsNew, err)
	}
	if !allowed {
		return fmt.Errorf("expected the all-namespaces credential to %q %q in namespace %q created after minting, but it was denied", verb, resource, nsNew)
	}
	return nil
}

// ThenNoManagedObjectsCreatedAnywhere asserts a refused attempt created no managed objects
// anywhere in the cluster — across all namespaces AND the cluster-scoped kinds (FR-018). It
// compares against the baseline captured before the attempt, so it is robust to managed
// objects left by earlier scenarios and would still catch a leaked ClusterRoleBinding.
func (s *SessionDSL) ThenNoManagedObjectsCreatedAnywhere(ctx context.Context) error {
	n, err := s.driver.CountAllManaged(ctx)
	if err != nil {
		return err
	}
	if n != s.baselineManaged {
		return fmt.Errorf("expected no new managed objects anywhere (baseline %d), found %d", s.baselineManaged, n)
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

// GivenNoActiveSessions establishes the zero-managed-objects precondition for the
// empty-inventory case. ls reads cluster-wide, so it directly purges every managed object
// (DeleteNamespace is async and gc only reaps the expired) — see PurgeAllManaged.
func (s *SessionDSL) GivenNoActiveSessions(ctx context.Context) error {
	s.driver.PurgeAllManaged(ctx)
	return nil
}

// WhenOperatorListsSessionsJSON lists active sessions in machine-readable form. It
// preserves any session-id minted earlier in the scenario (the ls run carries no
// session identity of its own) so ThenInventoryIncludesSession can match it.
func (s *SessionDSL) WhenOperatorListsSessionsJSON(ctx context.Context) error {
	s.mintedID = s.last.SessionID
	res, err := s.driver.LsJSON(ctx)
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}
	s.last = res
	return nil
}

// WhenOperatorPreviewsDryRun previews the accumulated request without creating anything.
func (s *SessionDSL) WhenOperatorPreviewsDryRun(ctx context.Context) error {
	s.req.Mode = drivers.ModeDryRun
	return s.mint(ctx)
}

// WhenLimitedOperatorMintsAllowedScope mints AS the limited identity requesting EXACTLY
// the read-only scope it is permitted (so the scope pre-flight passes) — the mint must
// then fail at the create-permission gate, not the scope gate (FR-016).
func (s *SessionDSL) WhenLimitedOperatorMintsAllowedScope(ctx context.Context, resource string) error {
	s.req.Verbs = []string{"get", "list", "watch"}
	s.req.Resources = []string{resource}
	s.req.AsIdentity = limitedIdentity
	s.req.Mode = drivers.ModePrintKubeconfig
	return s.mint(ctx)
}

// ThenInventoryIsEmpty asserts the machine-readable inventory is an empty list.
func (s *SessionDSL) ThenInventoryIsEmpty() error {
	if s.last.ExitCode != 0 {
		return fmt.Errorf("expected ls to exit 0, got %d: %s", s.last.ExitCode, strings.TrimSpace(s.last.Stderr))
	}
	out := strings.TrimSpace(s.last.Stdout)
	if out != "[]" {
		return fmt.Errorf("expected an empty inventory %q, got: %q", "[]", out)
	}
	return nil
}

// ThenInventoryIncludesActiveSession asserts the inventory parses as JSON and contains
// the session minted earlier, with a non-empty owner and expiry.
func (s *SessionDSL) ThenInventoryIncludesActiveSession() error {
	if s.last.ExitCode != 0 {
		return fmt.Errorf("expected ls to exit 0, got %d: %s", s.last.ExitCode, strings.TrimSpace(s.last.Stderr))
	}
	if s.mintedID == "" {
		return fmt.Errorf("no session was minted earlier in the scenario; nothing to look for")
	}
	var entries []inventoryEntry
	if err := json.Unmarshal([]byte(s.last.Stdout), &entries); err != nil {
		return fmt.Errorf("expected the inventory to parse as JSON, got %q: %w", strings.TrimSpace(s.last.Stdout), err)
	}
	for _, e := range entries {
		if e.SessionID == s.mintedID {
			if e.Owner == "" || e.ExpiresAt == "" {
				return fmt.Errorf("inventory entry for %q has empty owner/expiry: %+v", s.mintedID, e)
			}
			return nil
		}
	}
	return fmt.Errorf("expected the inventory to include session %q, got: %q", s.mintedID, strings.TrimSpace(s.last.Stdout))
}

// ThenIntendedObjectsDescribed asserts the dry-run preview named the objects it would
// create on the primary output (stdout) — service account, role and binding.
func (s *SessionDSL) ThenIntendedObjectsDescribed() error {
	if s.last.ExitCode != 0 {
		return fmt.Errorf("expected the dry run to exit 0, got %d: %s", s.last.ExitCode, strings.TrimSpace(s.last.Stderr))
	}
	out := strings.ToLower(s.last.Stdout)
	for _, want := range []string{"serviceaccount", "role", "binding"} {
		if !strings.Contains(out, want) {
			return fmt.Errorf("expected the dry-run preview to name the intended %q on the primary output, got: %q", want, strings.TrimSpace(s.last.Stdout))
		}
	}
	return nil
}

// ThenMissingCreatePermissionReported asserts the operator was told which create
// permission is missing — a clear, actionable message, not a raw API forbidden error.
func (s *SessionDSL) ThenMissingCreatePermissionReported() error {
	if !regexp.MustCompile(`(?i)missing verb: create on \w+`).MatchString(s.last.Stderr) {
		return fmt.Errorf("expected a 'missing verb: create on <resource>' message on the diagnostic output, got: %s", strings.TrimSpace(s.last.Stderr))
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
