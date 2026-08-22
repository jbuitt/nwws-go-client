package nwwsclient

import (
	"log/slog"
	"sync"
	"time"

	"gosrc.io/xmpp"
	"gosrc.io/xmpp/stanza"

	"github.com/jbuitt/nwws-go-client/internal/config"
	"github.com/jbuitt/nwws-go-client/internal/pan"
	"github.com/jbuitt/nwws-go-client/internal/product"
	"github.com/jbuitt/nwws-go-client/internal/store"
)

const mucRoom = "nwws@conference.nwws-oi.weather.gov"

// mucJID returns the full MUC occupant JID for the given resource/nickname.
func mucJID(resource string) string {
	return mucRoom + "/" + resource
}

const (
	backoffBase = 1 * time.Second
	backoffCap  = 60 * time.Second
)

// nextBackoff returns the delay before reconnect attempt number attempt
// (0-indexed): 1s, 2s, 4s, ... capped at 60s.
func nextBackoff(attempt int) time.Duration {
	d := backoffBase
	for i := 0; i < attempt; i++ {
		d *= 2
		if d >= backoffCap {
			return backoffCap
		}
	}
	return d
}

// Client runs the NWWS-OI connection: connecting, joining the MUC room,
// handling incoming products, and reconnecting on drop per cfg.Retry.
type Client struct {
	cfg    config.Config
	logger *slog.Logger
	panLog *slog.Logger

	panWG sync.WaitGroup
}

// New creates a Client. panLog is the logger used for PAN script output —
// it may be the same as logger when no dedicated PAN log file is configured.
func New(cfg config.Config, logger, panLog *slog.Logger) *Client {
	return &Client{cfg: cfg, logger: logger, panLog: panLog}
}

// handleMessage is the router handler for incoming "message" stanzas. It
// parses the nwws-oi product, saves it, and (if configured) launches the PAN
// script asynchronously. It never returns an error: parse/save failures are
// logged and the message is dropped, since the receive loop must keep going.
func (c *Client) handleMessage(_ xmpp.Sender, p stanza.Packet) {
	msg, ok := p.(stanza.Message)
	if !ok {
		return
	}

	prod, err := product.ParseMessage(msg)
	if err != nil {
		c.logger.Warn("skipping unparseable message", slog.Any("error", err))
		return
	}

	result, path, err := store.WriteProduct(c.cfg.ArchiveDir, prod)
	if err != nil {
		c.logger.Error("failed to save product",
			slog.String("cccc", prod.CCCC), slog.String("id", prod.ID), slog.Any("error", err))
		return
	}
	if result == store.DuplicateSkipped {
		c.logger.Info("skipped duplicate product", slog.String("path", path))
		return
	}
	c.logger.Info("saved product", slog.String("path", path))

	if c.cfg.PanRun != "" {
		c.panWG.Add(1)
		go func() {
			defer c.panWG.Done()
			pan.Run(c.panLog, c.cfg.PanRun, path)
		}()
	}
}

// waitForPAN blocks until all in-flight PAN goroutines finish, or until
// timeout elapses, whichever comes first.
func (c *Client) waitForPAN(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		c.panWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		c.logger.Warn("timed out waiting for in-flight PAN scripts")
	}
}
