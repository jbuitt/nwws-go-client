package nwwsclient

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gosrc.io/xmpp/stanza"

	"github.com/jbuitt/nwws-go-client/internal/config"
)

func TestNextBackoff(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 32 * time.Second},
		{6, 60 * time.Second},
		{7, 60 * time.Second},
		{20, 60 * time.Second},
	}
	for _, c := range cases {
		got := nextBackoff(c.attempt)
		if got != c.want {
			t.Errorf("nextBackoff(%d) = %v, want %v", c.attempt, got, c.want)
		}
	}
}

func TestMucJID(t *testing.T) {
	got := mucJID("nwws-go-client-abc12")
	want := "nwws@conference.nwws-oi.weather.gov/nwws-go-client-abc12"
	if got != want {
		t.Errorf("mucJID(...) = %q, want %q", got, want)
	}
}

const testMessageXML = `<message from="nwws@conference.nwws-oi.weather.gov/KKCI" to="user@nwws-oi.weather.gov/res">
  <x xmlns="nwws-oi" cccc="KKCI" ttaaii="FTUS21" awipsid="TAFKORD" issue="2026-08-22T14:32:00Z" id="99">HANDLER TEST TEXT</x>
</message>`

func TestHandleMessage_SavesProduct(t *testing.T) {
	dir := t.TempDir()
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	c := New(config.Config{ArchiveDir: dir}, logger, logger)

	var msg stanza.Message
	if err := xml.Unmarshal([]byte(testMessageXML), &msg); err != nil {
		t.Fatalf("unmarshaling test message: %v", err)
	}

	c.handleMessage(nil, msg)

	wantPath := filepath.Join(dir, "kkci", "kkci_ftus21-tafkord.221432_99.txt")
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("expected product file at %s, got error: %v", wantPath, err)
	}
	if string(data) != "HANDLER TEST TEXT" {
		t.Errorf("file content = %q, want HANDLER TEST TEXT", string(data))
	}
}

func TestHandleMessage_IgnoresNonMessagePackets(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	c := New(config.Config{ArchiveDir: dir}, logger, logger)

	c.handleMessage(nil, stanza.Presence{})

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no files to be written for a non-message packet, found %d", len(entries))
	}
}

func TestRunCancelable_ReturnsFnResultWhenFnFinishesFirst(t *testing.T) {
	err := runCancelable(context.Background(), func() error {
		return errors.New("boom")
	}, nil)
	if err == nil || err.Error() != "boom" {
		t.Errorf("runCancelable() = %v, want boom", err)
	}
}

func TestRunCancelable_ReturnsCtxErrWhenFnHangsForever(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := runCancelable(ctx, func() error {
		select {} // simulates client.Connect() blocking forever on an
		// unresponsive server, since the underlying XMPP library sets no
		// read deadline on the connection beyond the initial TCP dial.
	}, nil)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("runCancelable() error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > time.Second {
		t.Errorf("runCancelable() took %v to return, want it to return promptly once ctx is done, not wait for fn", elapsed)
	}
}

func TestRunCancelable_CallsAfterAbandonedOnlyOnceFnReturns(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	release := make(chan struct{})
	got := make(chan error, 1)
	fnReturned := make(chan struct{})
	err := runCancelable(ctx, func() error {
		<-release
		close(fnReturned)
		return errors.New("late result")
	}, func(err error) {
		select {
		case <-fnReturned: // cleanup must not run while fn is still running
		default:
			t.Error("afterAbandoned ran before fn returned")
		}
		got <- err
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("runCancelable() = %v, want context.DeadlineExceeded", err)
	}
	select {
	case <-got:
		t.Fatal("afterAbandoned called while fn was still blocked")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case e := <-got:
		if e == nil || e.Error() != "late result" {
			t.Errorf("afterAbandoned got %v, want fn's result", e)
		}
	case <-time.After(time.Second):
		t.Fatal("afterAbandoned never called after fn returned")
	}
}

func TestWaitForPAN_ReturnsWhenWorkDone(t *testing.T) {
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	c := New(config.Config{ArchiveDir: dir}, logger, logger)

	c.inflight.add()
	go func() {
		time.Sleep(10 * time.Millisecond)
		c.inflight.done()
	}()

	start := time.Now()
	c.waitForPAN(2 * time.Second)
	if time.Since(start) > time.Second {
		t.Error("waitForPAN took far longer than the in-flight work needed")
	}
}

func TestTracker_WaitReturnsImmediatelyWhenIdle(t *testing.T) {
	var tr tracker
	if !tr.wait(time.Second) {
		t.Error("wait() = false on an idle tracker, want true")
	}
}

func TestTracker_WaitTimesOutWhileWorkOutstanding(t *testing.T) {
	var tr tracker
	tr.add()
	if tr.wait(50 * time.Millisecond) {
		t.Error("wait() = true with work outstanding, want false after timeout")
	}
	tr.done()
	if !tr.wait(time.Second) {
		t.Error("wait() = false after work finished, want true")
	}
}

// add() racing with wait() is the case sync.WaitGroup forbids; run it under
// -race to prove tracker tolerates it.
func TestTracker_AddRacingWithWait(t *testing.T) {
	var tr tracker
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			tr.add()
			time.Sleep(time.Millisecond)
			tr.done()
		}()
		go func() {
			defer wg.Done()
			tr.wait(time.Second)
		}()
	}
	wg.Wait()
	if !tr.wait(time.Second) {
		t.Error("tracker not idle after all work finished")
	}
}
