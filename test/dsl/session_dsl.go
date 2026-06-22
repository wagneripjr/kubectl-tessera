//go:build e2e

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

type SessionDSL struct {
	driver   *drivers.KindDriver
	req      drivers.MintRequest
	last     drivers.MintResult
	mintedID string

	requestedNamespaces  []string
	unrequestedNamespace string

	baselineManaged int
}

type inventoryEntry struct {
	SessionID string `json:"sessionID"`
	Owner     string `json:"owner"`
	ExpiresAt string `json:"expiresAt"`
}

func New(driver *drivers.KindDriver) *SessionDSL {
	return &SessionDSL{driver: driver}
}

func (s *SessionDSL) GivenOperatorRequests(verbs, resource string) {
	s.req = drivers.MintRequest{
		Verbs:     splitCSV(verbs),
		Resources: []string{resource},
		TTL:       15 * time.Minute,
		Mode:      drivers.ModePrintKubeconfig,
	}
}

func (s *SessionDSL) GivenOperatorRequestsClusterScoped(verbs, resource string) {
	s.req = drivers.MintRequest{
		Verbs:         splitCSV(verbs),
		Resources:     []string{resource},
		ClusterScoped: true,
		TTL:           15 * time.Minute,
		Mode:          drivers.ModePrintKubeconfig,
	}
}

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

func (s *SessionDSL) GivenOperatorRequestsAllNamespaces(verbs, resource string) {
	s.req = drivers.MintRequest{
		Verbs:         splitCSV(verbs),
		Resources:     []string{resource},
		AllNamespaces: true,
		TTL:           15 * time.Minute,
		Mode:          drivers.ModePrintKubeconfig,
	}
}

func (s *SessionDSL) GivenOperatorRequestsNamed(verb, resource, name string) {
	s.req = drivers.MintRequest{
		Verbs:         []string{verb},
		Resources:     []string{resource},
		ResourceNames: []string{name},
		TTL:           15 * time.Minute,
		Mode:          drivers.ModePrintKubeconfig,
	}
}

func (s *SessionDSL) GivenLimitedOperator(ctx context.Context, resource string) error {
	if err := s.driver.SeedLimitedIdentity(ctx, limitedIdentity,
		[]string{"get", "list", "watch"}, []string{resource}, nil); err != nil {
		return fmt.Errorf("seeding limited identity: %w", err)
	}
	s.req.AsIdentity = limitedIdentity
	return nil
}

func (s *SessionDSL) GivenSubMinimumSession(ctx context.Context, resource string) error {
	s.req = drivers.MintRequest{
		Verbs:     []string{"get", "list", "watch"},
		Resources: []string{resource},
		TTL:       5 * time.Minute,
		Mode:      drivers.ModePrintKubeconfig,
	}
	return s.mint(ctx)
}

func (s *SessionDSL) GivenInteractiveSession(ctx context.Context, resource string) error {
	s.req = drivers.MintRequest{
		Verbs:     []string{"get", "list", "watch"},
		Resources: []string{resource},
		TTL:       15 * time.Minute,
		Mode:      drivers.ModeExec,
	}
	return s.mint(ctx)
}

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

func (s *SessionDSL) GivenExpiredManagedSession(ctx context.Context, sessionID string) error {
	return s.driver.SeedManagedSession(ctx, sessionID, time.Now().Add(-time.Hour))
}

func (s *SessionDSL) GivenUnexpiredManagedSession(ctx context.Context, sessionID string) error {
	return s.driver.SeedManagedSession(ctx, sessionID, time.Now().Add(time.Hour))
}

func (s *SessionDSL) GivenUnmanagedRoleBinding(ctx context.Context, name string) error {
	return s.driver.SeedUnmanagedRBAC(ctx, name)
}

func (s *SessionDSL) WhenOperatorMints(ctx context.Context) error { return s.mint(ctx) }

func (s *SessionDSL) WhenLimitedOperatorRequests(ctx context.Context, verb, resource string) error {
	s.req.Verbs = []string{verb}
	s.req.Resources = []string{resource}
	s.req.AsIdentity = limitedIdentity
	s.req.Mode = drivers.ModePrintKubeconfig
	return s.mint(ctx)
}

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

func (s *SessionDSL) WhenOperatorMintsPrintKubeconfig(ctx context.Context) error {
	s.req.Mode = drivers.ModePrintKubeconfig
	return s.mint(ctx)
}

func (s *SessionDSL) WhenOperatorLeavesSession(_ context.Context) error {
	return nil
}

func (s *SessionDSL) WhenSessionTerminatedAbruptly(ctx context.Context) error {
	return s.driver.KillExecProcess(ctx, "SIGKILL")
}

func (s *SessionDSL) WhenSessionLifetimeElapses(_ context.Context) error {
	time.Sleep(3 * time.Second)
	return nil
}

func (s *SessionDSL) WhenGarbageCollectionRuns(ctx context.Context) error {
	res, err := s.driver.Gc(ctx)
	if err != nil {
		return err
	}

	res.SessionID = s.last.SessionID
	res.KubeconfigPath = s.last.KubeconfigPath
	s.last = res
	return nil
}

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

func (s *SessionDSL) ThenMintRefused() error {
	if s.last.ExitCode == 0 {
		return fmt.Errorf("expected the mint to be refused (non-zero exit), got exit 0")
	}
	return nil
}

func (s *SessionDSL) ThenAllowedDeniedReported() error {
	out := s.last.Stdout + s.last.Stderr
	if !regexp.MustCompile(`(?i)(denied|allowed)`).MatchString(out) {
		return fmt.Errorf("expected an allowed/denied report, got: %s", strings.TrimSpace(out))
	}
	return nil
}

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

func (s *SessionDSL) ThenWarnedTTLFloored() error {
	if !strings.Contains(s.last.Stderr, "floored to cluster minimum") {
		return fmt.Errorf("expected a floor warning on the diagnostic output, got: %s", strings.TrimSpace(s.last.Stderr))
	}
	return nil
}

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

func (s *SessionDSL) ThenKubeconfigGrantsReadAccess(ctx context.Context, resource string) error {
	return s.ThenMintedCredentialCan(ctx, "is allowed", "get", resource, "")
}

func (s *SessionDSL) ThenAuditOnDiagnosticOutput() error {
	if !strings.Contains(s.last.Stderr, "session") {
		return fmt.Errorf("expected audit details on the diagnostic output (stderr), got: %s", strings.TrimSpace(s.last.Stderr))
	}
	return nil
}

func (s *SessionDSL) GivenNoActiveSessions(ctx context.Context) error {
	s.driver.PurgeAllManaged(ctx)
	return nil
}

func (s *SessionDSL) WhenOperatorListsSessionsJSON(ctx context.Context) error {
	s.mintedID = s.last.SessionID
	res, err := s.driver.LsJSON(ctx)
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}
	s.last = res
	return nil
}

func (s *SessionDSL) WhenOperatorPreviewsDryRun(ctx context.Context) error {
	s.req.Mode = drivers.ModeDryRun
	return s.mint(ctx)
}

func (s *SessionDSL) WhenLimitedOperatorMintsAllowedScope(ctx context.Context, resource string) error {
	s.req.Verbs = []string{"get", "list", "watch"}
	s.req.Resources = []string{resource}
	s.req.AsIdentity = limitedIdentity
	s.req.Mode = drivers.ModePrintKubeconfig
	return s.mint(ctx)
}

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

func (s *SessionDSL) ThenMissingCreatePermissionReported() error {
	if !regexp.MustCompile(`(?i)missing verb: create on \w+`).MatchString(s.last.Stderr) {
		return fmt.Errorf("expected a 'missing verb: create on <resource>' message on the diagnostic output, got: %s", strings.TrimSpace(s.last.Stderr))
	}
	return nil
}

func (s *SessionDSL) mint(ctx context.Context) error {
	res, err := s.driver.Mint(ctx, s.req)
	if err != nil {
		return fmt.Errorf("invoking mint: %w", err)
	}
	s.last = res
	return nil
}

func splitCSV(s string) []string { return strings.Split(s, ",") }
