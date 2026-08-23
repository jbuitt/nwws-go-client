# nwws-go-client

A console client for NOAA's NWWS-OI (NOAA Weather Wire Service) that joins
the `nwws@conference.nwws-oi.weather.gov` XMPP MUC room, parses broadcast
weather products, and saves them to `products/<cccc>/`.

## Build

    go build -o nwws-go-client ./cmd/nwws-go-client

This project depends on a locally vendored, lightly patched fork of
`gosrc.io/xmpp` at `third_party/gosrc.io-xmpp/` (wired in via a `replace`
directive in `go.mod`) — see that directory's `stanza/parser.go` (search for
`PATCHED`) and the design spec's "Resolved issue" section for what's
different from upstream and why. It's part of this repo, so a normal `go
build`/`go get` needs no extra setup.

## Configure

Configuration is resolved from, in order of priority (highest wins):

1. CLI flags
2. Environment variables
3. A JSON config file (see `config.example.json`)
4. Built-in defaults

| Flag             | Env var             | JSON key      | Default                          |
|------------------|----------------------|---------------|-----------------------------------|
| `-server`        | `NWWS_SERVER`        | `server`      | `nwws-oi.weather.gov`             |
| `-port`          | `NWWS_PORT`          | `port`        | `5222`                            |
| `-username`      | `NWWS_USERNAME`      | `username`    | *(required)*                      |
| `-password`      | `NWWS_PASSWORD`      | `password`    | *(required)*                      |
| `-resource`      | `NWWS_RESOURCE`      | `resource`    | `nwws-go-client-XXXXX` (random)   |
| `-archivedir`    | `NWWS_ARCHIVEDIR`    | `archivedir`  | `./products/`                     |
| `-pan_run`       | `NWWS_PAN_RUN`       | `pan_run`     | *(disabled)*                      |
| `-pan_run_log`   | `NWWS_PAN_RUN_LOG`   | `pan_run_log` | *(logs to stdout)*                |
| `-retry`         | `NWWS_RETRY`         | `retry`       | `true`                            |
| `-use_tls`       | `NWWS_USE_TLS`       | `use_tls`     | `true`                            |
| `-config <path>` | —                    | —             | `./config.json` (optional if absent) |

## Run

    ./nwws-go-client -username myuser -password mypass

Or with a config file:

    cp config.example.json config.json
    # edit config.json
    ./nwws-go-client

Stop with Ctrl+C (SIGINT) or SIGTERM — the client leaves the MUC room and
disconnects cleanly before exiting.

## Product Arrival Notification (PAN)

If `pan_run` is set, it's invoked as `pan_run <path-to-saved-product-file>`
after each newly-saved (non-duplicate) product, asynchronously with a 30s
timeout. Its output goes to `pan_run_log` if set, otherwise to the main log.

## Troubleshooting

`-debug_xmpp_log <path>` writes the raw XMPP wire traffic to a file, useful
for diagnosing connection/MUC-join problems. It's CLI-flag-only (no env var
or JSON key) since it's meant for one-off diagnostic runs, not something to
leave enabled persistently.

**The resulting file contains your base64-encoded SASL login exchange** —
treat it as sensitive, don't commit it or paste it in full anywhere. When
sharing it for troubleshooting, share only the portion from after the
`connecting to NWWS-OI` / auth-success point onward (e.g. everything from
the outgoing `<presence>` MUC-join stanza forward).

If you hit an `"unknown namespace ..."` error right after `"joining MUC
room"`: this was previously a real, confirmed bug — a non-conforming client
elsewhere in the MUC room sends its presence with an incorrectly-declared
namespace, and the underlying XMPP library rejected the whole connection
over it. It's fixed via the vendored fork mentioned above, which tolerates
this for any of `message`/`presence`/`iq`. If you still see it on a current
build, capture `-debug_xmpp_log` and see the design spec's "Resolved issue"
section for how the original was diagnosed — it may be a different,
not-yet-seen malformation.

## Author

[jbuitt at gmail.com](mailto:jbuitt@gmail.com)

## License

See [LICENSE](https://github.com/jbuitt/nwws-go-client/blob/master/LICENSE) file.
