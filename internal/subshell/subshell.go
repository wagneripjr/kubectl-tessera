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

const cleanupTimeout = 30 * time.Second

type Config struct {
	Shell  string
	Env    []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	Cleanup func(ctx context.Context)
}

func Run(parent context.Context, cfg Config) (exitCode int, err error) {
	sigCtx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	runCleanup := sync.OnceFunc(func() {
		if cfg.Cleanup == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		cfg.Cleanup(ctx)
	})
	defer runCleanup()

	cmd := exec.CommandContext(sigCtx, cfg.Shell)
	cmd.Env = cfg.Env
	cmd.Stdin = cfg.Stdin
	cmd.Stdout = cfg.Stdout
	cmd.Stderr = cfg.Stderr

	if startErr := cmd.Start(); startErr != nil {
		return -1, fmt.Errorf("starting shell %q: %w", cfg.Shell, startErr)
	}

	waitErr := cmd.Wait()

	runCleanup()

	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return -1, fmt.Errorf("waiting on shell: %w", waitErr)
	}
	return 0, nil
}
