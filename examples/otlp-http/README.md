# otlp-http

Exports metrics over **OTLP/HTTP** (protobuf over HTTPS) to a hosted backend or an OTLP gateway that authenticates with a bearer token. OTLP/HTTP is the most widely accepted OTLP transport and passes through ordinary HTTP proxies and load balancers.

## Settings

| Key | Value | Why |
| --- | ----- | --- |
| `[tags] env` | `production` | Sent as a resource attribute with every export, for grouping on the server. |
| `protocol` | `http` | OTLP/HTTP, default port 4318 (hosted services usually use 443). |
| `endpoint` | `https://otlp.example.com` | `https://` enables TLS. With no path, `/v1/metrics` is appended. |
| `compression` | `gzip` | Cuts payload size several-fold. |
| `timeout` | `10s` | Per request. |
| `interval` | `30s` | Buffered cycles are sent every 30 seconds. |
| `headers.authorization` | `Bearer …` | Replace with your token. |

## Setup

```bash
cp examples/otlp-http/config.toml ~/.config/metricsd/config.toml
chmod 600 ~/.config/metricsd/config.toml
$EDITOR ~/.config/metricsd/config.toml      # set endpoint and token
systemctl --user restart metricsd
journalctl --user -u metricsd -n 5          # "otlp export enabled"
```

## Backend-specific endpoints

Most hosted backends document an OTLP/HTTP endpoint and an auth header. Common shapes:

| Backend style | `endpoint` | Header |
| ------------- | ---------- | ------ |
| Standard OTLP gateway | `https://otlp.example.com` | `authorization = "Bearer <token>"` |
| Custom path | `https://ingest.example.com/otlp/v1/metrics` (used as written) | per vendor |
| Basic auth | `https://otlp.example.com` | `authorization = "Basic <base64 user:token>"` |
| API-key header | `https://otlp.example.com` | e.g. `x-api-key = "<key>"` |

Header names are case-insensitive. Add as many as the backend needs.

## Keep the token out of the file

Leave `[export.otlp.headers]` out and set the standard variable in a systemd drop-in instead:

```bash
systemctl --user edit metricsd
```

```ini
[Service]
Environment=OTEL_EXPORTER_OTLP_HEADERS=authorization=Bearer%20REPLACE_ME
```

Values in `OTEL_EXPORTER_OTLP_HEADERS` are URL-encoded (`%20` for a space). Headers in the config file take precedence over the variable.

## Verify

```bash
journalctl --user -u metricsd -f
```

Failed deliveries log `export failed; data is buffered locally` with the error, then retry with backoff. Data is kept in the local database and sent once the endpoint accepts it again, as far back as `retention`.

More: [docs/export.md](../../docs/export.md).
