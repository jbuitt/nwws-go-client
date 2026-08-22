# nwws-go-client

A console client for NOAA's NWWS-OI (NOAA Weather Wire Service) that joins
the `nwws@conference.nwws-oi.weather.gov` XMPP MUC room, parses broadcast
weather products, and saves them to `products/<cccc>/`.

## Build

    go build -o nwws-go-client ./cmd/nwws-go-client

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
