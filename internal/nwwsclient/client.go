package nwwsclient

import (
	"context"
	"fmt"
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

// Run connects to the NWWS-OI server, joins the MUC room, and processes
// incoming products until ctx is cancelled. It reconnects on connection
// loss according to cfg.Retry. It returns nil on a clean shutdown (ctx
// cancelled) or an error if the connection was lost and Retry is false.
func (c *Client) Run(ctx context.Context) error {
	if !c.cfg.UseTLS {
		c.logger.Warn("use_tls is disabled; connecting without STARTTLS")
	}

	router := xmpp.NewRouter()
	router.HandleFunc("message", c.handleMessage)

	errCh := make(chan error, 1)
	reportErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}

	xmppCfg := xmpp.Config{
		TransportConfiguration: xmpp.TransportConfiguration{
			Address: fmt.Sprintf("%s:%d", c.cfg.Server, c.cfg.Port),
		},
		Jid:        fmt.Sprintf("%s@%s/%s", c.cfg.Username, c.cfg.Server, c.cfg.Resource),
		Credential: xmpp.Password(c.cfg.Password),
		Insecure:   !c.cfg.UseTLS,
	}

	client, err := xmpp.NewClient(&xmppCfg, router, reportErr)
	if err != nil {
		return fmt.Errorf("configuring xmpp client: %w", err)
	}

	joinMUC := func() error {
		c.logger.Info("joining MUC room", slog.String("room", mucRoom))
		return client.Send(stanza.Presence{
			Attrs: stanza.Attrs{To: mucJID(c.cfg.Resource)},
			Extensions: []stanza.PresExtension{
				stanza.MucPresence{History: stanza.History{MaxStanzas: stanza.NewNullableInt(0)}},
			},
		})
	}
	client.PostConnectHook = joinMUC

	shutdown := func() {
		c.logger.Info("shutting down, leaving MUC room")
		_ = client.Send(stanza.Presence{
			Attrs: stanza.Attrs{To: mucJID(c.cfg.Resource), Type: stanza.StanzaType("unavailable")},
		})
		_ = client.Disconnect()
		c.waitForPAN(5 * time.Second)
	}

	c.logger.Info("connecting to NWWS-OI", slog.String("server", c.cfg.Server), slog.Int("port", c.cfg.Port))
	if err := client.Connect(); err != nil {
		return fmt.Errorf("connecting to %s:%d: %w", c.cfg.Server, c.cfg.Port, err)
	}

	attempt := 0
	for {
		select {
		case <-ctx.Done():
			shutdown()
			return nil

		case connErr := <-errCh:
			c.logger.Warn("xmpp connection error", slog.Any("error", connErr))
			if !c.cfg.Retry {
				c.waitForPAN(5 * time.Second)
				return fmt.Errorf("disconnected from NWWS-OI server: %w", connErr)
			}

			delay := nextBackoff(attempt)
			attempt++
			c.logger.Info("reconnecting to NWWS-OI", slog.Duration("delay", delay), slog.Int("attempt", attempt))
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				shutdown()
				return nil
			}

			if err := client.Connect(); err != nil {
				c.logger.Error("reconnect attempt failed", slog.Any("error", err))
				reportErr(err)
				continue
			}
			c.logger.Info("reconnected to NWWS-OI")
			attempt = 0
		}
	}
}
