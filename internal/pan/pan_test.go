package pan

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing script: %v", err)
	}
	return path
}

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, nil))
}

func TestRun_Success(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "success.sh", `echo "ran: $1"; exit 0`)

	var buf bytes.Buffer
	Run(newTestLogger(&buf), script, "/tmp/products/KKCI/some-file.txt")

	out := buf.String()
	if !strings.Contains(out, "PAN script completed") {
		t.Errorf("log output = %q, want it to mention success", out)
	}
	if !strings.Contains(out, "ran: /tmp/products/KKCI/some-file.txt") {
		t.Errorf("log output = %q, want it to include the script's stdout", out)
	}
}

func TestRun_Failure(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "failure.sh", `echo "failing on purpose"; exit 1`)

	var buf bytes.Buffer
	Run(newTestLogger(&buf), script, "/tmp/products/KKCI/some-file.txt")

	out := buf.String()
	if !strings.Contains(out, "PAN script failed") {
		t.Errorf("log output = %q, want it to mention failure", out)
	}
}

func TestRun_Timeout(t *testing.T) {
	original := Timeout
	Timeout = 50 * time.Millisecond
	defer func() { Timeout = original }()

	dir := t.TempDir()
	script := writeScript(t, dir, "slow.sh", `sleep 5; exit 0`)

	var buf bytes.Buffer
	Run(newTestLogger(&buf), script, "/tmp/products/KKCI/some-file.txt")

	out := buf.String()
	if !strings.Contains(out, "PAN script failed") {
		t.Errorf("log output = %q, want it to report failure due to timeout", out)
	}
}

func TestRun_TimeoutKillsProcessGroup(t *testing.T) {
	original := Timeout
	// Note: a very short timeout (e.g. 50ms, as used by TestRun_Timeout)
	// isn't reliable here: in a sandboxed test environment, process
	// startup latency can eat most or all of that budget, killing the
	// script before it even reaches its first line - which would make
	// this test pass vacuously regardless of whether the fix works. 300ms
	// gives the script room to actually fork its background child before
	// the timeout fires, so the test exercises the real scenario.
	Timeout = 300 * time.Millisecond
	defer func() { Timeout = original }()

	dir := t.TempDir()
	markerPath := filepath.Join(dir, "marker")
	script := writeScript(t, dir, "spawner.sh", `(sleep 1; touch "$1.marker") &
sleep 5`)

	var buf bytes.Buffer
	Run(newTestLogger(&buf), script, markerPath)

	// The background child sleeps 1s before touching the marker file. If
	// the fix works, Run() kills the whole process group when the timeout
	// fires, so the backgrounded child never gets to touch the marker
	// file, no matter how long we wait afterward. If it doesn't work, the
	// child is merely orphaned, keeps running past its parent's death, and
	// will have written the marker by the time we check (note: without the
	// fix, Run() itself can also block well past Timeout here, since the
	// orphan inherits Run()'s stdout/stderr pipe and keeps it open - see
	// https://go.dev/issue/23019 - so this wait mostly just needs to be
	// comfortably past the child's 1s sleep).
	time.Sleep(1500 * time.Millisecond)

	if _, err := os.Stat(markerPath + ".marker"); err == nil {
		t.Error("marker file exists: background child process survived the timeout and was not killed")
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected error checking marker file: %v", err)
	}
}
