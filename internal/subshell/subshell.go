// Package subshell spawns an interactive shell with an injected environment and
// guarantees a one-shot cleanup runs exactly once — on normal shell exit OR on
// SIGINT/SIGTERM delivered to this process. SIGKILL bypasses cleanup by design
// (the gc sweep reclaims orphans). It imports only the standard library.
package subshell

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// cleanupTimeout bounds the teardown so a wedged API server cannot hang the exit,
// while staying generous enough for foreground-propagation deletes to complete.
const cleanupTimeout = 30 * time.Second

// Config describes one subshell session.
type Config struct {
	Shell  string    // resolved shell path, e.g. ${SHELL:-/bin/bash}
	Env    []string  // full environment for the child (already includes KUBECONFIG=...)
	Stdin  io.Reader // typically os.Stdin
	Stdout io.Writer // typically os.Stdout
	Stderr io.Writer // typically os.Stderr

	// Cleanup runs exactly once, synchronously, before Run returns — on normal
	// shell exit and on SIGINT/SIGTERM. It receives a FRESH context derived from
	// context.Background(), so the signal that triggered teardown never cancels it.
	Cleanup func(ctx context.Context)
}

// Run spawns cfg.Shell, waits for it to exit (or for SIGINT/SIGTERM to this
// process), runs cfg.Cleanup exactly once, and returns the shell's exit code.
//
//   - parent drives shell termination only: SIGINT/SIGTERM (or a cancelled parent)
//     ends the shell. It never reaches Cleanup's context.
//   - exitCode is the child's exit status (0 on clean exit, -1 if it never ran).
//   - err is non-nil only for a genuine spawn/exec failure, NOT a non-zero shell
//     exit (that is reported via exitCode).
func Run(parent context.Context, cfg Config) (exitCode int, err error) {
	// sigCtx is cancelled by SIGINT/SIGTERM (or a cancelled parent). It drives only
	// the shell's lifetime — exec.CommandContext kills the child when it fires.
	sigCtx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Cleanup is reachable via normal exit and via signal cancellation, but must run
	// exactly once and against a context that the terminating signal cannot cancel.
	runCleanup := sync.OnceFunc(func() {
		if cfg.Cleanup == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		cfg.Cleanup(ctx)
	})
	defer runCleanup() // safety net for the spawn-failure and panic paths

	cmd := exec.CommandContext(sigCtx, cfg.Shell)
	cmd.Env = cfg.Env
	cmd.Stdin = cfg.Stdin
	cmd.Stdout = cfg.Stdout
	cmd.Stderr = cfg.Stderr

	if startErr := cmd.Start(); startErr != nil {
		return -1, fmt.Errorf("starting shell %q: %w", cfg.Shell, startErr)
	}

	waitErr := cmd.Wait()

	// Run cleanup synchronously before returning, so the process does not exit until
	// the session is torn down. The deferred call above is then a no-op (OnceFunc).
	runCleanup()

	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			return exitErr.ExitCode(), nil // non-zero / signal exit is data, not error
		}
		return -1, fmt.Errorf("waiting on shell: %w", waitErr)
	}
	return 0, nil
}
