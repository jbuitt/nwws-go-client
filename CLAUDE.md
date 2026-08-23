# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A Go CLI (`nwws-go-client`) that connects to NOAA's NWWS-OI XMPP service, joins the `nwws@conference.nwws-oi.weather.gov` MUC room, parses broadcast weather products, and saves them to `products/<cccc>/`. Optionally runs a PAN (Product Arrival Notification) script per saved product.

Full design rationale and history live in `docs/superpowers/specs/2026-08-22-nwws-client-design.md` (includes dated amendments explaining every deviation from the original design, several driven by real bugs found against the live server) and `docs/superpowers/plans/2026-08-22-nwws-client.md` (task-by-task implementation plan). Read the spec's amendments before assuming any XMPP-library behavior — several early hypotheses were wrong and corrected only after tracing the actual library source or capturing live server traffic.

## Build & Test Commands

```bash
go build -o nwws-go-client ./cmd/nwws-go-client       # build
go run ./cmd/nwws-go-client -username u -password p   # run without building
go test ./...                                         # all tests
go test ./internal/config/...                         # one package
go test ./internal/config/... -run TestLoad_Precedence -v  # one test
go test ./... -race                                    # with race detector
go vet ./...
go install ./cmd/nwws-go-client                        # install to $GOPATH/bin
```

Requires Go 1.22+ (uses `math/rand/v2`, `log/slog`); the toolchain pinned in `go.mod` is 1.27. No lint config beyond `go vet`; `gofmt -l .` (excluding `third_party/`) checks formatting. There is no CI config in this repo — the commands above are the whole verification loop.

## Code Style Guidelines

- Standard `gofmt` formatting; no project-specific style beyond it.
- Doc comments on exported identifiers explain *why*, not what — e.g. `waitForPAN`'s comment explains the WaitGroup race it narrows and why it can't close it fully, not "waits for PAN to finish." Match this: a comment restating the signature is noise, one is only worth adding when it captures a non-obvious constraint or reason.
- Errors are wrapped with `fmt.Errorf("...: %w", err)` and logged via `log/slog` with structured fields (`slog.String(...)`, `slog.Any("error", err)`), never with `fmt.Println`/bare `log`. Errors are only silently discarded (`_ = ...`) at a few deliberate, comment-justified points (e.g. best-effort presence sends during shutdown) — don't add a new one without the same justification.
- Table-driven tests for pure-logic functions (see `internal/nwwsclient/client_test.go`'s `TestNextBackoff` or `internal/config`'s precedence tests). Tests that touch the filesystem use `t.TempDir()`; tests that touch env vars use `t.Setenv()`.
- No premature abstraction: config's ~10 fields are handled with explicit per-field code across three layers (flag/env/JSON) rather than a reflection- or table-driven generic merge — a deliberate, reviewed choice given the field count is small and stable. Prefer this project's existing bias toward explicit, repeated code over cleverness when extending it.
- Keep each `internal/` package's exported surface minimal — unexported helpers (`sanitizeID`, `mucJID`, `nextBackoff`, `runCancelable`, `nwwsExtension`) stay unexported even when a package grows; only what other packages actually need is exported.

## Core Architecture

`cmd/nwws-go-client/main.go` is pure wiring: `config.Load` → `nwwsclient.New` → `Client.Run(ctx)`, with `signal.NotifyContext` for graceful shutdown. All real logic lives in five independent `internal/` packages, each usable and testable without the others:

- **`internal/config`**: `Config` struct + `Load()`. Precedence is CLI flag > env var > JSON file > default, resolved explicitly via a `map[string]bool` from `flag.Visit` — not zero-value comparison, since an explicitly-passed `-retry=false` must still beat a `true` default. `-debug_xmpp_log` is deliberately CLI-flag-only (no env/JSON) since it's a one-off diagnostic, not persistent config.
- **`internal/product`**: `ParseMessage(stanza.Message) (Product, error)` extracts the `nwws-oi` XML extension. `Filename()`/`Dir()` build the archive path — lowercase, with all four untrusted upstream fields (`cccc`, `ttaaii`, `awipsid`, `id`) sanitized against path traversal (a real vulnerability was found and fixed here: only `id` was originally sanitized, not the other three, which come from the same untrusted source). A `nwws_processor` routing tag sometimes prepended to `id` upstream is replaced with `0000`.
- **`internal/store`**: `WriteProduct` uses `O_CREATE|O_EXCL` for atomic, race-free duplicate detection — a file that already exists is left untouched, not overwritten.
- **`internal/pan`**: `Run` executes the PAN script with a 30s timeout, killing the entire process group on timeout (not just the direct child — a real bug: a PAN script's own spawned subprocesses would otherwise outlive the timeout).
- **`internal/nwwsclient`**: the connection lifecycle. `handleMessage` (the XMPP router's per-stanza handler) does parse → store → optionally launch PAN async, tracked via a `sync.WaitGroup` so shutdown can drain in-flight PAN scripts. `Run()` owns connect/MUC-join/reconnect-backoff/graceful-shutdown.

Reconnection backoff is 1s/2s/4s/.../60s-capped, resets on successful reconnect. `Retry=false` skips reconnect entirely and exits non-zero on disconnect. The initial connection attempt is *not* covered by `Retry` — only disconnects after a successful first connect trigger the retry loop.

### Vendored XMPP library fork — read before touching anything XMPP-related

`go.mod` has `replace gosrc.io/xmpp => ./third_party/gosrc.io-xmpp`, a locally patched copy of `gosrc.io/xmpp` (`FluuxIO/go-xmpp`) v0.5.1, not the real dependency. The only change is in `third_party/gosrc.io-xmpp/stanza/parser.go` (search `PATCHED`): `NextPacket` falls back to local-name dispatch (`message`/`presence`/`iq`) when a stanza's top-level namespace isn't recognized, instead of hard-failing the whole connection. This exists because a real MUC occupant on the live NWWS-OI feed sends malformed presence (wrong namespace on the outer element), and the room replays every occupant's presence on every join — without the patch the client enters a permanent reconnect loop. **If a dependency bump ever touches `gosrc.io/xmpp`, this patch must be manually reapplied to the new version** — it lives in a separate local module tree and will not carry over automatically.

Several other `gosrc.io/xmpp` behaviors are non-obvious and already solved; don't rediscover them:
- The library has no MUC-join helper. Joining is a `stanza.Presence` sent to the room JID with a `stanza.MucPresence{}` extension attached; leaving is a presence of type `"unavailable"`.
- `Client.Resume()` redials and rebinds the session but **never restarts the `recv()`/`keepalive()` goroutines** — those only start inside `Client.Connect()`. The reconnect loop therefore calls `Connect()` again on every reconnect, never `Resume()`.
- `Client.Connect()` sets no read deadline beyond the initial TCP dial's `ConnectTimeout`; if the server hangs mid-handshake it blocks forever. `runCancelable()` races it against the shutdown context so Ctrl+C stays responsive.
- The library silently *drops* any XML sub-element it doesn't have a registered extension type for — this is how the `nwws-oi` product payload was almost lost during development. Any new stanza extension needs `stanza.TypeRegistry.MapExtension(...)` called in an `init()` before the first message is parsed (see `internal/product/product.go`).
- `Event.State` (connection-state introspection) has unexported internals and is unusable from outside the library package. The only externally observable disconnect signal is the `errorHandler func(error)` callback passed to `xmpp.NewClient`.
