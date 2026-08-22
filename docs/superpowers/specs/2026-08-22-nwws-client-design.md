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

`internal/config.Config` fields (all from the spec): `Server`, `Port`,
`Username`, `Password`, `Resource`, `ArchiveDir`, `PanRun`, `PanRunLog`,
`Retry`, `UseTLS`, `UseSSL`.

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
| UseSSL | `false` |

### Env var names

`NWWS_SERVER`, `NWWS_PORT`, `NWWS_USERNAME`, `NWWS_PASSWORD`, `NWWS_RESOURCE`,
`NWWS_ARCHIVEDIR`, `NWWS_PAN_RUN`, `NWWS_PAN_RUN_LOG`, `NWWS_RETRY`,
`NWWS_USE_TLS`, `NWWS_USE_SSL`.

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
  "use_tls": true,
  "use_ssl": false
}
```

### Startup validation

Missing `username` or `password` after resolving all sources is a fatal
error: log it and exit with status 1 before attempting to connect.

## XMPP connection

Uses `fluuxio/go-xmpp`. Connects to `<server>:<port>`, authenticates, and
joins the MUC room `nwws@conference.nwws-oi.weather.gov` using the resolved
`Resource` as the MUC nickname.

- `UseSSL=true` → implicit TLS: wrap the socket in TLS before the XMPP
  handshake begins (no STARTTLS negotiation).
- `UseSSL=false, UseTLS=true` (default) → connect in plaintext then negotiate
  STARTTLS.
- Both false → unencrypted connection is attempted; a WARN is logged since
  this is discouraged.

The exact `go-xmpp` `Options` fields used to express implicit-TLS vs. STARTTLS
vs. plaintext will be mapped to whatever that library's API actually exposes
at implementation time; the behavioral intent above is the contract.

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

If `Retry=true` (default) and the XMPP connection drops:

- Reconnect with exponential backoff: 1s, 2s, 4s, 8s, ... capped at 60s.
- Backoff resets to 1s after a successful reconnect + MUC rejoin.
- Each attempt and failure is logged.

If `Retry=false`, a dropped connection is logged as an ERROR and the process
exits with a non-zero status.

## Graceful shutdown

On SIGINT/SIGTERM:

1. Cancel the root context (stops the receive loop and any pending
   reconnect/backoff wait).
2. Send MUC "unavailable" presence and close the XMPP connection.
3. Wait up to 5 seconds for in-flight PAN goroutines to finish.
4. Exit 0.

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
