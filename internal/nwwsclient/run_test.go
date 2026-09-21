package nwwsclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jbuitt/nwws-go-client/internal/config"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func waitForFile(path string, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// A reconnect after a server-side drop must behave exactly like a first
// connect: negotiate STARTTLS before authenticating, rejoin the MUC, and go
// on receiving products. gosrc.io/xmpp v0.5.1 keeps its TLS-secured flag and
// Session on the reused Client, so reconnecting the same Client skipped
// STARTTLS and attempted a plaintext login.
func TestRun_ReconnectRenegotiatesTLSAndKeepsReceiving(t *testing.T) {
	srv := newMockXMPP(t)
	dir := t.TempDir()

	logs := &syncBuf{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	cfg := config.Config{
		Server: "127.0.0.1", Port: srv.port(),
		Username: "u", Password: "p", Resource: "res",
		ArchiveDir: dir, Retry: true, UseTLS: true,
	}
	c := New(cfg, logger, logger)
	c.tlsConfig = &tls.Config{InsecureSkipVerify: true}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()

	first := filepath.Join(dir, "kkci", "kkci_ftus21-tafkord.221432_conn0.txt")
	second := filepath.Join(dir, "kkci", "kkci_ftus21-tafkord.221432_conn1.txt")

	if !waitForFile(first, 10*time.Second) {
		t.Fatalf("product from first connection never saved\nlogs:\n%s", logs.String())
	}
	if !waitForFile(second, 15*time.Second) {
		t.Errorf("product from the reconnected session never saved\nlogs:\n%s", logs.String())
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v after cancel, want nil", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Run did not return after cancel")
	}

	for i, r := range srv.records() {
		if r.authSeen && !r.authSecure {
			t.Errorf("connection %d sent SASL credentials in plaintext (STARTTLS was skipped)", i)
		}
		if r.authSeen && !r.startTLS {
			t.Errorf("connection %d authenticated without negotiating STARTTLS", i)
		}
	}
}

// gosrc.io/xmpp sets no read deadline after the TCP dial, so a server that
// accepts a reconnect and then goes silent used to block the client for
// minutes (observed: ~3.5 min per attempt). The client must abandon such an
// attempt after connectTimeout, back off, and recover on the next one.
func TestRun_ReconnectRecoversFromStalledAttempt(t *testing.T) {
	srv := newMockXMPP(t, 1) // connection 0 works, 1 stalls, 2 works
	dir := t.TempDir()

	logs := &syncBuf{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	cfg := config.Config{
		Server: "127.0.0.1", Port: srv.port(),
		Username: "u", Password: "p", Resource: "res",
		ArchiveDir: dir, Retry: true, UseTLS: true,
	}
	c := New(cfg, logger, logger)
	c.tlsConfig = &tls.Config{InsecureSkipVerify: true}
	c.connectTimeout = 300 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()

	third := filepath.Join(dir, "kkci", "kkci_ftus21-tafkord.221432_conn2.txt")
	if !waitForFile(third, 15*time.Second) {
		t.Errorf("client never recovered after a stalled reconnect attempt\nlogs:\n%s", logs.String())
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v after cancel, want nil", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	if !strings.Contains(logs.String(), "timed out after") {
		t.Errorf("expected the stalled attempt to be logged as timed out\nlogs:\n%s", logs.String())
	}
}
