package subshell_test

import (
	"context"
	"testing"

	"github.com/wagneripjr/kubectl-tessera/internal/subshell"
)

func TestNormalShellExitRunsCleanup(t *testing.T) {
	cleaned := 0
	exitCode, err := subshell.Run(context.Background(), subshell.Config{
		Shell:   "/usr/bin/true",
		Cleanup: func(context.Context) { cleaned++ },
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exit code: got %d, want 0", exitCode)
	}
	if cleaned != 1 {
		t.Fatalf("cleanup ran %d times, want exactly 1", cleaned)
	}
}

func TestNonZeroShellExitStillRunsCleanup(t *testing.T) {
	cleaned := 0
	exitCode, err := subshell.Run(context.Background(), subshell.Config{
		Shell:   "/usr/bin/false",
		Cleanup: func(context.Context) { cleaned++ },
	})
	if err != nil {
		t.Fatalf("non-zero shell exit must not be a Go error, got: %v", err)
	}
	if exitCode != 1 {
		t.Fatalf("exit code: got %d, want 1", exitCode)
	}
	if cleaned != 1 {
		t.Fatalf("cleanup ran %d times, want exactly 1", cleaned)
	}
}

func TestCleanupRunsWithLiveContextWhenParentCancelled(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	cleaned := 0
	var cleanupCtxErr error
	_, _ = subshell.Run(parent, subshell.Config{
		Shell:   "/usr/bin/true",
		Cleanup: func(ctx context.Context) { cleaned++; cleanupCtxErr = ctx.Err() },
	})

	if cleaned != 1 {
		t.Fatalf("cleanup ran %d times, want exactly 1", cleaned)
	}
	if cleanupCtxErr != nil {
		t.Fatalf("cleanup received a cancelled context (%v); it must get a live one", cleanupCtxErr)
	}
}

func TestSpawnFailureReturnsErrorAndRunsCleanup(t *testing.T) {
	cleaned := 0
	_, err := subshell.Run(context.Background(), subshell.Config{
		Shell:   "/nonexistent/shell-does-not-exist",
		Cleanup: func(context.Context) { cleaned++ },
	})

	if err == nil {
		t.Fatal("expected an error spawning a nonexistent shell, got nil")
	}
	if cleaned != 1 {
		t.Fatalf("cleanup ran %d times, want exactly 1", cleaned)
	}
}
