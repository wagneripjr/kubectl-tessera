//go:build e2e

// Package e2e wires the Gherkin acceptance specs (specs/features) to the DSL via
// godog. Run with: go test -tags=e2e ./test/e2e/...  (requires a reachable kind
// cluster). Select scenarios with GODOG_TAGS (default excludes @manual). See
// ADR-009, ADR-010.
package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/cucumber/godog"

	"github.com/wagneripjr/kubectl-tessera/test/drivers"
	"github.com/wagneripjr/kubectl-tessera/test/dsl"
)

// Shared across the suite. godog runs scenarios sequentially by default, so a
// package-level driver + per-scenario DSL is safe.
var (
	driver  *drivers.KindDriver
	current *dsl.SessionDSL
	scnNS   string
)

func TestFeatures(t *testing.T) {
	tags := os.Getenv("GODOG_TAGS")
	if tags == "" {
		tags = "~@manual" // exclude the webhook-authorizer scenario by default (ADR-011)
	}

	suite := godog.TestSuite{
		Name:                 "tessera-acceptance",
		TestSuiteInitializer: initializeSuite,
		ScenarioInitializer:  initializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../specs/features"},
			TestingT: t,
			Strict:   true,
			Tags:     tags,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("acceptance suite failed (expected RED until features are implemented)")
	}
}

func initializeSuite(sc *godog.TestSuiteContext) {
	sc.BeforeSuite(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		d, err := drivers.NewKindDriver(ctx)
		if err != nil {
			// A broken harness is NOT a valid RED — fail loudly and stop.
			panic(fmt.Sprintf("harness setup failed (build binary / reach kind): %v", err))
		}
		driver = d
	})
	sc.AfterSuite(func() {
		if driver != nil {
			driver.Close()
		}
	})
}

func initializeScenario(ctx *godog.ScenarioContext) {
	ctx.Before(func(c context.Context, sc *godog.Scenario) (context.Context, error) {
		scnNS = namespaceFor(sc)
		driver.SetNamespace(scnNS)
		if err := driver.EnsureNamespace(c, scnNS); err != nil {
			return c, err
		}
		current = dsl.New(driver)
		return c, nil
	})
	ctx.After(func(c context.Context, sc *godog.Scenario, _ error) (context.Context, error) {
		// Best-effort teardown: sweep + drop the namespace (cascades namespaced RBAC).
		_, _ = driver.Gc(c)
		_ = driver.DeleteNamespace(c, scnNS)
		return c, nil
	})

	registerSteps(ctx)
}

// registerSteps binds Gherkin steps to the DSL. Every step is a CLOSURE that reads
// the package-level `current` at CALL time. Do NOT register method values
// (current.Method): godog runs the ScenarioInitializer before the Before hook sets
// `current`, so a method value would bind to the previous scenario's DSL (or nil).
func registerSteps(ctx *godog.ScenarioContext) {
	// scope_enforcement.feature
	ctx.Step(`^an operator requests "([^"]*)" on "([^"]*)" in the session namespace$`,
		func(verbs, resource string) error { current.GivenOperatorRequests(verbs, resource); return nil })
	ctx.Step(`^an operator requests "([^"]*)" on the cluster-scoped resource "([^"]*)"$`,
		func(verbs, resource string) error { current.GivenOperatorRequestsClusterScoped(verbs, resource); return nil })
	ctx.Step(`^an operator requests "([^"]*)" on "([^"]*)" named "([^"]*)" in the session namespace$`,
		func(verb, resource, name string) error { current.GivenOperatorRequestsNamed(verb, resource, name); return nil })
	ctx.Step(`^the operator mints the session$`,
		func(c context.Context) error { return current.WhenOperatorMints(c) })
	ctx.Step(`^the minted credential (is allowed|is not allowed) to "([^"]*)" "([^"]*)"$`,
		func(c context.Context, outcome, verb, resource string) error {
			return current.ThenMintedCredentialCan(c, outcome, verb, resource, "")
		})
	ctx.Step(`^the minted credential (is allowed|is not allowed) to "([^"]*)" the "([^"]*)" named "([^"]*)"$`,
		func(c context.Context, outcome, verb, resource, name string) error {
			return current.ThenMintedCredentialCan(c, outcome, verb, resource, name)
		})

	// preflight_gate.feature
	ctx.Step(`^a limited operator who may only read "([^"]*)"$`,
		func(c context.Context, resource string) error { return current.GivenLimitedOperator(c, resource) })
	ctx.Step(`^the limited operator requests "([^"]*)" on "([^"]*)" in the session namespace$`,
		func(c context.Context, verb, resource string) error { return current.WhenLimitedOperatorRequests(c, verb, resource) })
	ctx.Step(`^the mint is refused$`, func() error { return current.ThenMintRefused() })
	ctx.Step(`^the allowed and denied parts of the requested scope are reported$`,
		func() error { return current.ThenAllowedDeniedReported() })
	ctx.Step(`^no managed objects are created for that attempt$`,
		func(c context.Context) error { return current.ThenNoManagedObjectsCreated(c) })

	// lifecycle_cleanup.feature
	ctx.Step(`^an operator mints a read-only session requesting a lifetime below the cluster minimum$`,
		func(c context.Context) error { return current.GivenSubMinimumSession(c, "pods") })
	ctx.Step(`^the minted credential works immediately$`,
		func(c context.Context) error { return current.ThenMintedCredentialWorks(c, "pods") })
	ctx.Step(`^the operator is warned that the lifetime was floored to the cluster minimum$`,
		func() error { return current.ThenWarnedTTLFloored() })
	ctx.Step(`^the session lifetime elapses$`, func(c context.Context) error { return current.WhenSessionLifetimeElapses(c) })
	ctx.Step(`^an operator mints an interactive read-only session$`,
		func(c context.Context) error { return current.GivenInteractiveSession(c, "pods") })
	ctx.Step(`^the operator leaves the interactive session$`, func(c context.Context) error { return current.WhenOperatorLeavesSession(c) })
	ctx.Step(`^the session's managed objects are gone$`, func(c context.Context) error { return current.ThenSessionObjectsGone(c) })
	ctx.Step(`^the session kubeconfig file is removed$`, func() error { return current.ThenKubeconfigFileRemoved() })
	ctx.Step(`^the session process is terminated abruptly$`, func(c context.Context) error { return current.WhenSessionTerminatedAbruptly(c) })
	ctx.Step(`^the garbage-collection sweep runs$`, func(c context.Context) error { return current.WhenGarbageCollectionRuns(c) })
	ctx.Step(`^object creation will fail partway through$`, func(c context.Context) error { return current.GivenCreationWillFail(c) })
	ctx.Step(`^the operator mints a session$`, func(c context.Context) error { return current.WhenOperatorMints(c) })
	ctx.Step(`^no managed objects remain for that session$`, func(c context.Context) error { return current.ThenNoManagedObjectsRemain(c) })
	ctx.Step(`^an expired managed session exists$`,
		func(c context.Context) error { return current.GivenExpiredManagedSession(c, "expired1") })
	ctx.Step(`^an unexpired managed session exists$`,
		func(c context.Context) error { return current.GivenUnexpiredManagedSession(c, "fresh1") })
	ctx.Step(`^an unmanaged role binding exists$`,
		func(c context.Context) error { return current.GivenUnmanagedRoleBinding(c, "unmanaged-rb") })
	ctx.Step(`^the expired managed session is removed$`,
		func(c context.Context) error { return current.ThenManagedSessionRemoved(c, "expired1") })
	ctx.Step(`^the unexpired managed session remains$`,
		func(c context.Context) error { return current.ThenManagedSessionRemains(c, "fresh1") })
	ctx.Step(`^the unmanaged role binding remains$`,
		func(c context.Context) error { return current.ThenUnmanagedRoleBindingRemains(c, "unmanaged-rb") })

	// distribution_cli.feature
	ctx.Step(`^an operator requests a read-only "([^"]*)" session for non-interactive use$`,
		func(resource string) error { current.GivenOperatorRequests("get,list,watch", resource); return nil })
	ctx.Step(`^the operator mints the session in print-kubeconfig mode$`,
		func(c context.Context) error { return current.WhenOperatorMintsPrintKubeconfig(c) })
	ctx.Step(`^only the kubeconfig path is written to the primary output$`, func() error { return current.ThenOnlyPathOnStdout() })
	ctx.Step(`^the produced kubeconfig grants the requested read access$`,
		func(c context.Context) error { return current.ThenKubeconfigGrantsReadAccess(c, "pods") })
	ctx.Step(`^the session audit details are written only to the diagnostic output$`, func() error { return current.ThenAuditOnDiagnosticOutput() })

	// discovery.feature (@manual; excluded by default — pending until a webhook cluster exists)
	ctx.Step(`^a cluster whose authorizer cannot enumerate permissions$`, func() error { return godog.ErrPending })
	ctx.Step(`^the operator previews the scope with a dry run$`, func() error { return godog.ErrPending })
	ctx.Step(`^a "([^"]*)" warning is shown$`, func(string) error { return godog.ErrPending })
	ctx.Step(`^the preview does not claim to be exhaustive$`, func() error { return godog.ErrPending })
}

func namespaceFor(sc *godog.Scenario) string {
	// Derive a stable, DNS-1123 namespace from the scenario id.
	id := sc.Id
	const max = 40
	if len(id) > max {
		id = id[len(id)-max:]
	}
	out := make([]rune, 0, len(id))
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			out = append(out, r)
		default:
			out = append(out, '-')
		}
	}
	return "tessera-test-" + string(out)
}
