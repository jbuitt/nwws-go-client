// Package pan runs external "PAN" scripts after a product has been saved,
// notifying downstream tooling that a new file is available.
package pan

import (
	"bytes"
	"context"
	"log/slog"
	"os/exec"
	"syscall"
	"time"
)

// Timeout bounds how long a PAN script is allowed to run before it's
// killed. It's a var (not a const) so tests can shorten it.
var Timeout = 30 * time.Second

// Run executes panRun with filePath as its sole argument, logging its exit
// status and combined output via logger. Run is meant to be launched in its
// own goroutine by the caller so a slow script never blocks product
// ingestion; a caller that needs to wait for completion (e.g. during
// shutdown) should track that itself, such as with a sync.WaitGroup around
// the "go Run(...)" call.
func Run(logger *slog.Logger, panRun, filePath string) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, panRun, filePath)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	// Put the child in its own process group and, on timeout/cancellation,
	// kill the whole group rather than just the direct child. Without
	// this, exec.CommandContext's default cancellation only kills panRun
	// itself: if panRun forks a subprocess (backgrounds a task, shells out
	// to curl/mail, etc.) before the timeout fires, that subprocess is
	// reparented and keeps running indefinitely, undetected. This
	// contradicts the guarantee that a hung/slow PAN script can never
	// accumulate indefinitely.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	attrs := []any{
		slog.String("pan_run", panRun),
		slog.String("file", filePath),
		slog.Duration("duration", duration),
		slog.String("output", out.String()),
	}
	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
		logger.Error("PAN script failed", attrs...)
		return
	}
	logger.Info("PAN script completed", attrs...)
}
