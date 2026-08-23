# NOAA Weather Wire (NWWS-OI) Client — Design

Date: 2026-08-22
Module: `github.com/jbuitt/nwws-go-client`

## Purpose

A console/CLI Go program that connects to the public NOAA Weather Wire Service
(NWWS-OI) XMPP server, joins the `nwws@conference.nwws-oi.weather.gov` MUC
room, parses broadcasted weather products from the XML message stanzas, and
saves each product as a file under `products/<cccc>/`. Optionally runs a
Product Arrival Notification (PAN) script after each save.

## Non-goals

- No GUI. Console/CLI only.
- No live/mocked XMPP integration test harness in the initial implementation
  (unit tests cover the pure-logic pieces only).
- No product content transformation/enrichment beyond what's needed to derive
  the filename — the raw product text is written as-is.

## Architecture

```
cmd/nwws-go-client/main.go       — wiring: load config, start client, handle signals
internal/config/                 — Config struct, flag/env/JSON loading + precedence merge
internal/nwwsclient/             — go-xmpp wrapper: connect, join MUC, receive loop, reconnect/backoff
internal/product/                — Product struct, ParseStanza(), Filename()
internal/store/                  — WriteProduct(): directory creation, duplicate detection
internal/pan/                    — RunPAN(): async script execution + logging
config.example.json
go.mod
```

`go-xmpp`'s message callback delivers stanzas one at a time in order. Parsing
and file-writing happen synchronously inside that callback — no worker pools
or channels needed given NWWS's message rate. The one thing that can be slow
(the PAN script) runs in its own goroutine so it can never stall the receive
loop.

## Configuration

`internal/config.Config` fields: `Server`, `Port`, `Username`, `Password`,
`Resource`, `ArchiveDir`, `PanRun`, `PanRunLog`, `Retry`, `UseTLS`.

> **Amendment (2026-08-22):** `UseSSL` (implicit/direct TLS) was dropped
> entirely — no field, CLI flag, env var, or JSON key. Research into the
> chosen XMPP library (`FluuxIO/go-xmpp`, see "XMPP connection" below) showed
> it only supports STARTTLS; implicit TLS isn't reachable through its public
> API without forking it. Real NWWS-OI traffic runs over STARTTLS on port
> 5222, so this isn't a functional loss.

### Precedence (highest wins)

1. CLI flag, if explicitly passed (detected via `flag.Visit`, not zero-value
   checks — a flag explicitly set to a zero value must still win over env/JSON).
2. Environment variable, if set.
3. JSON config file value, if present.
4. Hardcoded default.

### Defaults

| Field | Default |
|---|---|
| Server | `nwws-oi.weather.gov` |
| Port | `5222` |
| Username | none (required) |
| Password | none (required) |
| Resource | `nwws-go-client-XXXXX` (random 5-char alphanumeric suffix) |
| ArchiveDir | `./products/` |
| PanRun | none (PAN disabled if unset) |
| PanRunLog | none (PAN output logged to main logger if unset) |
| Retry | `true` |
| UseTLS | `true` |

### Env var names

`NWWS_SERVER`, `NWWS_PORT`, `NWWS_USERNAME`, `NWWS_PASSWORD`, `NWWS_RESOURCE`,
`NWWS_ARCHIVEDIR`, `NWWS_PAN_RUN`, `NWWS_PAN_RUN_LOG`, `NWWS_RETRY`,
`NWWS_USE_TLS`.

### JSON config file

Located via `-config` flag, default `./config.json`. Missing file at the
*default* path is not an error (silently skipped). An explicitly-passed
`-config <path>` that doesn't exist is a fatal startup error.

Example (`config.example.json`):

```json
{
  "server": "nwws-oi.weather.gov",
  "port": 5222,
  "username": "[username]",
  "password": "[password]",
  "resource": "[resource]",
  "archivedir": "[archivedir]",
  "pan_run": "[pan_run]",
  "pan_run_log": "[pan_run_log]",
  "retry": true,
  "use_tls": true
}
```

### Startup validation

Missing `username` or `password` after resolving all sources is a fatal
error: log it and exit with status 1 before attempting to connect.

## XMPP connection

Uses `FluuxIO/go-xmpp` (module `gosrc.io/xmpp`, packages `gosrc.io/xmpp` and
`gosrc.io/xmpp/stanza`). Connects to `<server>:<port>`, authenticates, and
joins the MUC room `nwws@conference.nwws-oi.weather.gov` using the resolved
`Resource` as the MUC nickname (as the resource part of the room JID:
`nwws@conference.nwws-oi.weather.gov/<resource>`).

- `xmpp.Config.Insecure = !UseTLS`. When `UseTLS=true` (default), STARTTLS is
  required; when `false`, an unencrypted connection is allowed (a WARN is
  logged at startup since this is discouraged).
- There is no implicit-TLS option — see the `UseSSL` amendment above.
- Joining the MUC is not a single library call: the library has no dedicated
  MUC helper. It's done by sending a `stanza.Presence` to the room JID with a
  `stanza.MucPresence{}` extension attached (XEP-0045), done from the
  `Config.` `PostConnectHook`/`StreamManager.PostConnect` callback so it fires
  on every (re)connect. Leaving is a presence of type `unavailable` to the
  same room JID.
- Incoming messages arrive via a `Router` route registered with
  `router.HandleFunc("message", handler)`; the handler receives a
  `stanza.Message`.
- The NWWS-OI product data is a message extension in the `nwws-oi` namespace
  (`<x xmlns="nwws-oi" ...>`). `go-xmpp` silently drops any XML sub-element it
  doesn't recognize, so `internal/product` must register a custom
  `stanza.MsgExtension` type for that namespace in an `init()` function
  (`stanza.TypeRegistry.MapExtension(stanza.PKTMessage, xml.Name{Space:
  "nwws-oi", Local: "x"}, NWWSProduct{})`) before any message is received, or
  the product data is silently lost.

## Product parsing

Each incoming MUC message stanza is expected to carry a child element
`<x xmlns="nwws-oi">` with attributes:

- `cccc` — 4-letter originating office
- `ttaaii` — WMO report type/region/number code
- `awipsid` — AWIPS product ID
- `issue` — ISO 8601 issuance timestamp
- `id` — unique product ID

...and the raw product text as the element's character data.

`internal/product.ParseStanza(stanza)` extracts these into:

```go
type Product struct {
    CCCC      string
    TTAAII    string
    AWIPSID   string
    Issue     time.Time
    ID        string
    Text      string
}
```

If any required attribute is missing, or `issue` fails to parse as a
timestamp, log a WARN with whatever fields were available and **skip** the
stanza — the receive loop must never crash or block on malformed input.

## Filename generation

`Product.Filename()` returns:

```
[cccc]_[ttaaii]-[awipsid].[ddHHMM]_[id].txt
```

`ddHHMM` is derived from `Issue` in UTC. `ID` is sanitized to
`[A-Za-z0-9._-]` only before being embedded in a filename (defense against
unexpected characters from upstream).

> **Amendment (2026-08-23):** the whole filename, and the `[cccc]` directory
> name from `Dir()`, are lowercased (`strings.ToLower`) after assembly, per
> user request. Also, the literal substring `nwws_processor` occurring in
> `ID` (NWWS-OI sometimes prepends a routing/relay tag like
> `nwws_processor.2953`, which carries no product-identity information) is
> replaced with `0000` before sanitization, so e.g. id `nwws_processor.2953`
> becomes `0000.2953` in the filename.

## Storage

`internal/store.WriteProduct(archiveDir string, p Product) error`:

1. Ensures `<archiveDir>/<cccc>/` exists (`os.MkdirAll`, mode 0755).
2. Opens the target file with `O_CREATE|O_EXCL|O_WRONLY`.
3. On `os.ErrExist` (duplicate product already on disk): log an INFO
   "duplicate skipped" message and return without error — this is expected,
   not a failure.
4. On any other error (permissions, disk full, etc.): log ERROR and return
   the error to the caller (product is dropped, receive loop continues).

## PAN execution

If `PanRun` is set, after a successful (non-duplicate) write:

- Launch `exec.CommandContext(ctx, PanRun, fullFilePath)` in its own
  goroutine, with a 30-second timeout (chosen default; not specified by the
  user) so a hung script can never accumulate indefinitely.
- Capture combined stdout/stderr and the exit status; log them.
- If `PanRunLog` is set, PAN-specific log lines go to that file (a dedicated
  `slog` logger writing there) instead of the main stdout logger.
- Fire-and-forget: the receive loop does not wait for the PAN script before
  processing the next message. On graceful shutdown, the process waits up to
  5 seconds for in-flight PAN goroutines via a `sync.WaitGroup`, then proceeds
  regardless.

## Logging

`log/slog` at INFO/WARN/ERROR levels, writing to stdout by default. Logged
events include: connection attempts and results, MUC join, each product
received/saved/skipped-as-duplicate, PAN script results, and all errors
(connection, filesystem, parsing).

## Reconnection / retry

`go-xmpp` ships a `StreamManager` with its own built-in reconnect/backoff, but
its backoff (20ms base, full jitter, 3-minute cap) doesn't match the spec
below and it always retries unconditionally — it has no way to express
`Retry=false`. Its `EventHandler`/`Event.State` mechanism (the other
candidate for detecting disconnects) turns out to be unusable from outside
the library too: `Event.State` is a `SyncConnState` whose state field and
`getState()` accessor are both unexported, so external code has no way to
read which connection state actually occurred. So `internal/nwwsclient`
hand-rolls the reconnect loop using the one externally-observable signal the
library does provide — the `errorHandler func(error)` callback passed to
`xmpp.NewClient`, which fires whenever the receive loop's stanza decode
fails (i.e. on every disconnect):

- If `Retry=true` (default): reconnect with exponential backoff — 1s, 2s, 4s,
  8s, ... capped at 60s — calling `client.Resume()` each attempt. Backoff
  resets to 1s after a successful reconnect + MUC rejoin. Each attempt and
  failure is logged.
- If `Retry=false`: a dropped connection is logged as an ERROR and the
  process exits with a non-zero status instead of reconnecting.

> **Amendment (2026-08-22):** Reconnect attempts call `client.Connect()`, not
> `client.Resume()`. `Resume()` only redials and re-binds the session — it
> never starts the keepalive/recv goroutines that `Connect()` starts, and
> `recv()` is the only code path that reads stanzas and reports future
> errors. Reconnecting via `Resume()` meant the client silently stopped
> receiving all products after the first successful reconnect, with no way
> to even detect a subsequent disconnect. `Connect()` is safe to call again
> on the same client (the transport unconditionally redials with no guard
> against reuse), so it's the call that actually gets the client receiving
> messages again after a drop.

## Graceful shutdown

On SIGINT/SIGTERM:

1. Cancel the root context (stops the receive loop and any pending
   reconnect/backoff wait).
2. Send MUC "unavailable" presence and close the XMPP connection.
3. Wait up to 5 seconds for in-flight PAN goroutines to finish.
4. Exit 0.

> **Amendment (2026-08-23):** `client.Connect()` calls (both the initial
> connect and each reconnect attempt) are wrapped in a `runCancelable(ctx,
> client.Connect)` helper that races the call against `ctx.Done()`. This was
> added after a real user hit an unresponsive Ctrl+C: `gosrc.io/xmpp`'s
> connect/auth/bind path never sets a read deadline on the connection beyond
> the initial TCP dial's `ConnectTimeout` (confirmed by reading `session.go`
> — none of its `Decode` calls for stream features, SASL, or IQ bind/session
> call `SetReadDeadline`), so if the server stops responding mid-handshake,
> `client.Connect()` can block indefinitely with nothing to interrupt it.
> `runCancelable` abandons the blocked call and returns `ctx.Err()`
> immediately instead; the leaked goroutine is harmless since the whole
> process exits shortly after a cancelled `Run()` returns. On the shutdown
> path taken this way, `shutdown()` (which sends unavailable-presence and
> calls `client.Disconnect()`) is deliberately skipped in favor of just
> draining PAN goroutines, since calling `Disconnect()` while the abandoned
> `Connect()` goroutine might still be mutating the same transport object
> would risk a data race.

## Testing

Table-driven unit tests for the pure-logic packages:

- `internal/config`: precedence merging across all combinations of
  flag/env/JSON/default, including the "explicitly-set-to-zero-value" case.
- `internal/product`: valid stanzas, missing required attributes, malformed
  timestamps, sanitization of `id`.
- `internal/store`: duplicate detection using a temp directory, directory
  creation, permission-error propagation.

No live or mocked XMPP integration test in this iteration.

## Open assumptions (call out if wrong)

- Product stanza structure assumed to be the commonly documented NWWS-OI
  format (`<x xmlns="nwws-oi">` with `cccc`/`ttaaii`/`awipsid`/`issue`/`id`
  attributes). Not verified against a captured live stanza.
- PAN script timeout of 30 seconds is a reasonable default, not user-specified.
- PAN script invoked with the saved file's full path as its only argument.
- `go-xmpp`'s `Config.Domain` (used in the STARTTLS handshake's SNI/hostname
  verification) is assumed to equal the configured `Server` hostname — NWWS-OI
  doesn't use a separate XMPP domain from its connect host as far as could be
  determined without a live connection to verify.

## Resolved issue: "unknown namespace ... <presence/>" on MUC join (2026-08-23)

A real user with valid NWWS-OI credentials hit `"xmpp connection error"
error="unknown namespace http://jabber.org/protocol/muc <presence/>"`
immediately after `"joining MUC room"` logged, followed by a permanent
reconnect loop that never actually recovered.

**Investigation.** The error string was traced to `stanza.NextPacket`'s
top-level dispatch (`gosrc.io/xmpp@v0.5.1/stanza/parser.go`), which requires
a top-level stanza's namespace to be one of a small fixed set
(`jabber:client` etc.) and hard-fails the entire connection otherwise. Two
hypotheses were tested and ruled out before the real cause was found:
(1) a missing `PresExtension` registration for `http://jabber.org/protocol/muc#user`
causing decoder desync — disproven by feeding several realistic
join-confirmation reconstructions through the actual decoder, all of which
decoded cleanly; (2) the client's own `<history maxstanzas="0">` request (an
unrequested addition from Task 11, deviating from the reference
Python/slixmpp client) triggering a server-side quirk — removed as a cheap
experiment, but the live server still failed identically, ruling it out too
(the history-request removal was kept anyway, since matching the reference
client's plainer join request has no downside).

**Root cause, confirmed via live capture.** With the user's explicit
permission, a `-debug_xmpp_log` capture was taken against the real NWWS-OI
server. It showed the exact failing stanza: one MUC occupant (nickname
`listener`, evidently a bot/service account and a persistent room member)
broadcasts its presence as
`<presence xmlns="http://jabber.org/protocol/muc" ...>` — incorrectly
declaring the MUC namespace on the presence element itself, instead of only
on the inner `<x>` child, unlike every other of the ~30 occupants replayed
on join (various slixmpp/Smack/Openfire clients). This is a bug in that
occupant's own XMPP client, not something NWWS-OI or this project controls
— but because the room replays every current occupant's presence on every
join, and `listener` is apparently always present, this occurred on
essentially every connection attempt, permanently.

**Fix.** `gosrc.io/xmpp` offers no supported hook to recover from a single
malformed stanza without tearing down the whole connection (the failure
happens inside the library's own unexported `recv()` loop). So the project
now vendors a small, surgically patched fork of the library at
`third_party/gosrc.io-xmpp/`, wired in via a `replace` directive in `go.mod`.
The only change (`stanza/parser.go`, clearly marked `PATCHED`) makes
`NextPacket` fall back to dispatching by local name (`message`/`presence`/
`iq`) when the top-level namespace isn't recognized, rather than hard-erroring
— matching how other mature XMPP clients (slixmpp, Smack) tolerate this same
non-conformance. Verified against the exact captured malformed stanza (now
decodes cleanly) and against the live server end-to-end: the client stays
connected, joins the room, and continuously saves real weather products
without disconnecting.

**Maintenance implication:** upgrading `gosrc.io/xmpp` requires re-applying
this patch to the new version (it's a ~15-line, clearly isolated change, not
expected to be hard to reapply).
