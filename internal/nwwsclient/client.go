package nwwsclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
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

	// tlsConfig overrides the STARTTLS client config. Always nil in
	// production (system roots + hostname verification); tests set it to
	// trust a self-signed mock server.
	tlsConfig *tls.Config

	// connectTimeout bounds one connection attempt; tests shorten it.
	connectTimeout time.Duration

	// inflight tracks handleMessage calls and PAN scripts still running, so
	// shutdown can wait for them.
	inflight tracker
}

// New creates a Client. panLog is the logger used for PAN script output —
// it may be the same as logger when no dedicated PAN log file is configured.
func New(cfg config.Config, logger, panLog *slog.Logger) *Client {
	return &Client{cfg: cfg, logger: logger, panLog: panLog, connectTimeout: connectTimeout}
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

	// Count this call as in flight for its whole duration, not just the PAN
	// goroutine below: gosrc.io/xmpp spawns a goroutine per received stanza to
	// call this handler, so at shutdown a call may not have reached its PAN
	// launch yet. tracker (unlike sync.WaitGroup) allows add() to race with
	// wait(), so that is harmless. A stanza dispatched after shutdown has
	// already started waiting simply isn't waited for.
	c.inflight.add()
	defer c.inflight.done()

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
		c.inflight.add()
		go func() {
			defer c.inflight.done()
			pan.Run(c.panLog, c.cfg.PanRun, path)
		}()
	}
}

// waitForPAN blocks until all in-flight work finishes, or until timeout
// elapses, whichever comes first.
func (c *Client) waitForPAN(timeout time.Duration) {
	if !c.inflight.wait(timeout) {
		c.logger.Warn("timed out waiting for in-flight PAN scripts")
	}
}

// tracker counts in-flight work. It replaces sync.WaitGroup, whose contract
// forbids Add racing with Wait while the count is zero — a race the
// per-stanza goroutines gosrc.io/xmpp spawns would trip at shutdown (caught
// by the race detector in TestRun_ReconnectRenegotiatesTLSAndKeepsReceiving).
type tracker struct {
	mu   sync.Mutex
	n    int
	idle chan struct{} // non-nil while a waiter is parked; closed when n reaches 0
}

func (t *tracker) add() {
	t.mu.Lock()
	t.n++
	t.mu.Unlock()
}

func (t *tracker) done() {
	t.mu.Lock()
	t.n--
	if t.n == 0 && t.idle != nil {
		close(t.idle)
		t.idle = nil
	}
	t.mu.Unlock()
}

// wait reports whether the count reached zero within timeout.
func (t *tracker) wait(timeout time.Duration) bool {
	t.mu.Lock()
	if t.n == 0 {
		t.mu.Unlock()
		return true
	}
	if t.idle == nil {
		t.idle = make(chan struct{})
	}
	ch := t.idle
	t.mu.Unlock()

	select {
	case <-ch:
		return true
	case <-time.After(timeout):
		return false
	}
}

// runCancelable runs fn in a goroutine and returns its result, unless ctx is
// done first, in which case it returns ctx.Err() immediately without waiting
// for fn. This exists because gosrc.io/xmpp's Client.Connect() sets no read
// deadline beyond the initial TCP dial (none of session.go's Decode calls for
// stream features, SASL, or IQ bind/session call SetReadDeadline), so if the
// server goes quiet mid-handshake Connect() can block forever with no way to
// cancel it.
//
// An abandoned fn keeps running. If afterAbandoned is non-nil it is called
// (on its own goroutine) with fn's eventual result once fn returns, so the
// caller can release whatever fn was setting up — at a point where nothing
// else is touching it, which is why cleanup must not be done concurrently.
func runCancelable(ctx context.Context, fn func() error, afterAbandoned func(error)) error {
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		if afterAbandoned != nil {
			go func() { afterAbandoned(<-done) }()
		}
		return ctx.Err()
	}
}

// connectTimeout bounds one whole connection attempt (dial, STARTTLS, SASL,
// bind, MUC join). gosrc.io/xmpp sets no read deadline after the initial TCP
// dial, so a server that accepts the connection and then goes quiet would
// otherwise block Connect() indefinitely; a real reconnect attempt was
// observed hanging for over three minutes this way.
const connectTimeout = 60 * time.Second

// Run connects to the NWWS-OI server, joins the MUC room, and processes
// incoming products until ctx is cancelled. It reconnects on connection
// loss according to cfg.Retry. It returns nil on a clean shutdown (ctx
// cancelled) or an error if the connection was lost and Retry is false.
//
// Every connection attempt, initial or reconnect, uses a brand-new
// xmpp.Client. gosrc.io/xmpp v0.5.1 keeps per-connection state on the Client
// (the transport's TLS-secured flag, the Session with its TlsEnabled flag)
// that is never reset by a second Connect(); reusing one Client made a
// reconnect think the fresh plaintext socket was already encrypted, skip
// STARTTLS, and send SASL credentials in the clear. A fresh client shares no
// state with its predecessor, which also makes it safe to close the old one
// in the background.
func (c *Client) Run(ctx context.Context) error {
	if !c.cfg.UseTLS {
		c.logger.Warn("use_tls is disabled; connecting without STARTTLS")
	}

	router := xmpp.NewRouter()
	router.HandleFunc("message", c.handleMessage)

	var debugLog *os.File
	if c.cfg.DebugXMPPLog != "" {
		f, err := os.Create(c.cfg.DebugXMPPLog)
		if err != nil {
			return fmt.Errorf("opening debug_xmpp_log %q: %w", c.cfg.DebugXMPPLog, err)
		}
		defer f.Close()
		debugLog = f
		c.logger.Warn("raw XMPP wire traffic logging enabled; this file will contain your base64-encoded SASL credentials, treat it as sensitive",
			slog.String("path", c.cfg.DebugXMPPLog))
	}

	// gen identifies the live client. A client's error callback only counts
	// while its generation is current, so late errors from a dead or
	// abandoned client (e.g. the extra "unknown namespace urn:xmpp:ping"
	// error gosrc.io/xmpp emits from a background goroutine after a failed
	// connect) can't be mistaken for a failure of the healthy new client.
	var gen atomic.Int64
	errCh := make(chan error, 1)
	drainErrs := func() {
		for {
			select {
			case <-errCh:
			default:
				return
			}
		}
	}

	newClient := func() (*xmpp.Client, error) {
		myGen := gen.Add(1)
		xmppCfg := xmpp.Config{
			TransportConfiguration: xmpp.TransportConfiguration{
				Address:   fmt.Sprintf("%s:%d", c.cfg.Server, c.cfg.Port),
				TLSConfig: c.tlsConfig,
			},
			Jid:          fmt.Sprintf("%s@%s/%s", c.cfg.Username, c.cfg.Server, c.cfg.Resource),
			Credential:   xmpp.Password(c.cfg.Password),
			Insecure:     !c.cfg.UseTLS,
			StreamLogger: debugLog,
		}
		client, err := xmpp.NewClient(&xmppCfg, router, func(err error) {
			if gen.Load() != myGen {
				return
			}
			select {
			case errCh <- err:
			default:
			}
		})
		if err != nil {
			return nil, err
		}
		client.PostConnectHook = func() error {
			c.logger.Info("joining MUC room", slog.String("room", mucRoom))
			// No explicit <history> element: let the server use its own
			// default (matches the reference Python/slixmpp client's
			// join_muc()). stanza.MucPresence{} with a zero-value History
			// omits the <history> element from the marshaled XML entirely.
			return client.Send(stanza.Presence{
				Attrs:      stanza.Attrs{To: mucJID(c.cfg.Resource)},
				Extensions: []stanza.PresExtension{stanza.MucPresence{}},
			})
		}
		return client, nil
	}

	// connect makes one attempt with a fresh client, giving up after
	// connectTimeout or when ctx is cancelled.
	connect := func() (*xmpp.Client, error) {
		client, err := newClient()
		if err != nil {
			return nil, fmt.Errorf("configuring xmpp client: %w", err)
		}
		attemptCtx, cancel := context.WithTimeout(ctx, c.connectTimeout)
		defer cancel()
		// If the attempt is abandoned while Connect() is still blocked, the
		// client is closed only once Connect() returns (server answered,
		// errored, or hung up): Disconnect() concurrent with a running
		// Connect() would race on the transport's fields.
		err = runCancelable(attemptCtx, client.Connect, func(error) { _ = client.Disconnect() })
		if err == nil {
			return client, nil
		}
		// Retire this client: ignore whatever its background goroutines
		// report from here on, and discard anything already reported.
		gen.Add(1)
		drainErrs()
		if attemptCtx.Err() != nil && ctx.Err() == nil {
			err = fmt.Errorf("timed out after %s", c.connectTimeout)
		}
		return nil, err
	}

	c.logger.Info("connecting to NWWS-OI", slog.String("server", c.cfg.Server), slog.Int("port", c.cfg.Port))
	client, err := connect()
	if err != nil {
		if ctx.Err() != nil {
			return nil // shutdown requested while connecting
		}
		return fmt.Errorf("connecting to %s:%d: %w", c.cfg.Server, c.cfg.Port, err)
	}

	shutdown := func() {
		c.logger.Info("shutting down, leaving MUC room")
		_ = client.Send(stanza.Presence{
			Attrs: stanza.Attrs{To: mucJID(c.cfg.Resource), Type: stanza.StanzaType("unavailable")},
		})
		_ = client.Disconnect()
		c.waitForPAN(5 * time.Second)
	}

	attempt := 0
	for {
		select {
		case <-ctx.Done():
			shutdown()
			return nil

		case connErr := <-errCh:
			c.logger.Warn("xmpp connection error", slog.Any("error", connErr))
			// The client is dead: stop listening to it, and release its
			// socket in the background (Disconnect waits on the server).
			gen.Add(1)
			go client.Disconnect()
			if !c.cfg.Retry {
				c.waitForPAN(5 * time.Second)
				return fmt.Errorf("disconnected from NWWS-OI server: %w", connErr)
			}

			for {
				delay := nextBackoff(attempt)
				attempt++
				c.logger.Info("reconnecting to NWWS-OI", slog.Duration("delay", delay), slog.Int("attempt", attempt))
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					c.waitForPAN(5 * time.Second)
					return nil
				}

				next, err := connect()
				if err != nil {
					if ctx.Err() != nil {
						c.waitForPAN(5 * time.Second)
						return nil
					}
					c.logger.Error("reconnect attempt failed", slog.Any("error", err))
					continue
				}
				client = next
				attempt = 0
				c.logger.Info("reconnected to NWWS-OI")
				break
			}
		}
	}
}
