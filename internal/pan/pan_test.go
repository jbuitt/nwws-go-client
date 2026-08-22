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
