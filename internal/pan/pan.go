// Package pan runs external "PAN" scripts after a product has been saved,
// notifying downstream tooling that a new file is available.
package pan

import (
	"bytes"
	"context"
	"log/slog"
	"os/exec"
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
