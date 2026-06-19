# Design — ATDD Protocol Drivers

This is the canonical design for the acceptance-test protocol driver (ADR-009). The driver is
the **only** bridge between the executable specs (`specs/features`) and the system under test.
Specs and DSL speak business language; the driver speaks the SUT's real external protocols.

## Why a composite driver

Tessera has **two** external protocols whose joint behavior is the product:

1. **The CLI binary** — invoked as a subprocess (`kubectl-tessera …`), observed via stdout,
   stderr, exit code, and the kubeconfig file it writes.
2. **The resulting cluster state** — the managed RBAC objects, their labels/annotations, and
   crucially the **effective permissions of the minted token** (does it actually allow
   `get pods` and deny `delete pods`?).

A spec like "a read-only pods session can read but not delete pods" is only truthfully
verifiable by **exercising the minted credential against the real API server**, not by reading
the binary's own claims. So the driver is a composite: a **process adapter** that spawns the
binary plus a **cluster adapter** (client-go, admin context) that inspects reality and runs
`auth can-i` / real requests *with the minted token*. The faithful API server is **kind**.

## Interface (`test/drivers/driver.go`, `//go:build e2e`)

The driver exposes only domain terms — no `os/exec`, GVR, or HTTP details leak to the DSL. It
holds **zero assertions** (ATDD Gate G5); assertions live in `test/dsl`.

```go
type MintMode int
const ( ModeExec MintMode = iota; ModePrintKubeconfig; ModeDryRun )

// MintRequest — the operator's request in domain terms.
type MintRequest struct {
    Verbs, Resources, Namespaces, ResourceNames []string
    APIGroup       string
    TTL            time.Duration
    ClusterScoped  bool          // tessera --cluster-scoped
    Mode           MintMode
    AsIdentity     string        // "" = admin context; else a seeded limited identity
}

// MintResult — everything observable from ONE binary invocation.
type MintResult struct {
    ExitCode       int
    Stdout, Stderr string
    KubeconfigPath string        // parsed from stdout in ModePrintKubeconfig
    SessionID      string        // parsed from the stderr audit line or -o json
}

type TesseraDriver interface {
    // process adapter — drives the SUT through the real CLI
    Mint(ctx context.Context, req MintRequest) (MintResult, error)
    Gc(ctx context.Context) (MintResult, error)
    Ls(ctx context.Context) (MintResult, error)
    KillExecProcess(ctx context.Context, signal string) error // SIGKILL for crash recovery

    // cluster adapter — verifies real state (admin context)
    SessionObjectsExist(ctx context.Context, sessionID string) (bool, error)
    SessionObjectsCount(ctx context.Context, sessionID string) (int, error)
    UnmanagedRBACUntouched(ctx context.Context, fingerprint map[string]bool) (bool, error)

    // the load-bearing checks — effective authz of the MINTED token
    MintedTokenCan(ctx context.Context, kubeconfigPath, verb, resource, namespace, name string) (bool, error)
    MintedTokenRequest(ctx context.Context, kubeconfigPath, verb, resource, namespace string) (status int, err error)

    // harness helpers (admin context)
    SeedUnmanagedRBAC(ctx context.Context, name string) error
    SeedExpiredManagedSession(ctx context.Context, sessionID string) error
    KubeconfigFileExists(path string) (bool, error)

    Close()
}
```

### Two key distinctions

- **`MintedTokenCan` vs `MintedTokenRequest`.** `MintedTokenCan` runs an SSAR using the minted
  token's REST config (`auth can-i` semantics) — the precise way to assert "can get / cannot
  delete" (#1/#7/#8). For **TTL expiry** (#3) the SSAR itself may be rejected, so the assertion
  must be a *real* request via `MintedTokenRequest` (expect 401/403 after expiry). Build the
  minted REST config by loading the kubeconfig the binary wrote.
- **`Mint` returns `error` only for harness failures** (couldn't spawn). A non-zero *exit
  code* from the binary (the over-ask refusal, #2) is **data** in `MintResult.ExitCode`,
  asserted by the DSL — keeping the driver assertion-free.

## DSL (`test/dsl`)

Holds per-scenario state and **all** assertions, returning `error` (godog idiom — a returned
error fails the step). Example: `ThenTheMintedCredentialCannotDeletePods` calls
`MintedTokenCan(…, "delete", "pods", …)` and returns an error if it is allowed.

## Suite & scenario lifecycle (`test/e2e/features_test.go`)

- **BeforeSuite:** build the binary once; build the admin client from the kind kubeconfig;
  fail fast if the API server is unreachable (a broken harness, not a valid RED); seed the
  **limited identity** (`tessera-limited` SA + narrow Role + token + kubeconfig) used by #2/#8.
- **Before(scenario):** create `tessera-test-<slug>-<rand>` namespace + fixture objects.
- **After(scenario):** delete the namespace (cascades namespaced RBAC), run `tessera gc`, and
  delete cluster-scoped objects by `session-id` label selector (namespace deletion won't reach
  ClusterRole/Binding). kubeconfigs go in a per-scenario temp dir.

## Testability constraints this design imposes on the SUT

- **`--exec` must be drivable without a TTY** (e.g. honor a `SHELL` set to a script that exits)
  and expose the child PID so `KillExecProcess` can `SIGKILL` it (#5). Design exec this way
  from the start.
- **TTL specs** assert against the **returned** `ExpirationTimestamp` and wait until
  `now > expiry` rather than a hardcoded sleep, so they survive apiserver clamping. The e2e
  kind cluster is configured to allow short token expirations. Tagged `@slow`.
