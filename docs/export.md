# Exporting metrics (OTLP)

metricsd can send everything it collects to a central server over **OTLP**, the OpenTelemetry protocol, using either OTLP/HTTP or OTLP/gRPC. Any OTLP backend works: the OpenTelemetry Collector, Grafana (Mimir/LGTM), Datadog, New Relic, Elastic, Honeycomb, and others.

metricsd only gathers and delivers metrics. Organizing sources, dashboards, and alerting happen on the server.

## Our format, OTLP transport

Metrics keep metricsd's own names and labels (`cpu.usage_pct{core=all}`, `mem.used_bytes`, ...) all the way to the server. OTLP is only the transport. Exporters sit behind one small interface (`internal/export`), so other formats can be added later without changing collectors or storage.

## Enable it

```toml
[export.otlp]
enabled  = true
protocol = "grpc"                                  # or "http"
endpoint = "https://otel.example.com:4317"
```

Restart metricsd. The startup log confirms it:

```
level=INFO msg="otlp export enabled" protocol=grpc endpoint=https://otel.example.com:4317 interval=30s
```

## Settings

All keys live under `[export.otlp]`.

| Key           | Default  | Description |
| ------------- | -------- | ----------- |
| `enabled`     | `false`  | Turn export on |
| `protocol`    | `"http"` | `"http"` (OTLP/HTTP protobuf) or `"grpc"` |
| `endpoint`    | empty    | Server URL; see [Endpoints](#endpoints) |
| `headers`     | none     | Extra request headers (authentication); a TOML table |
| `compression` | `"gzip"` | `"gzip"` or `"none"` |
| `timeout`     | `"10s"`  | Per-request timeout |
| `interval`    | `"30s"`  | How often to send. Minimum `1s`. Independent of the collection `interval`. |
| `ca_file`     | empty    | PEM CA bundle to trust in addition to the system roots |
| `cert_file`   | empty    | Client certificate for mutual TLS (needs `key_file`) |
| `key_file`    | empty    | Client key for mutual TLS (needs `cert_file`) |

Top-level `[tags]` become resource attributes on every export; see [Identity](#identity-resource-attributes).

### Endpoints

| Endpoint value | Result |
| -------------- | ------ |
| `https://host:4317` | TLS, verified against the system trust store (plus `ca_file`) |
| `http://host:4318` | Plaintext. Use only on trusted networks (localhost, a VPN, Tailscale). |
| `http://host:4318` with `protocol = "http"` | Posted to `http://host:4318/v1/metrics`, since the standard path is added when the URL has none |
| `https://host/custom/path` with `protocol = "http"` | Posted to that exact path |
| empty | Taken from `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` or `OTEL_EXPORTER_OTLP_ENDPOINT`, falling back to `localhost:4318` (http) or `localhost:4317` (grpc) |

The standard OpenTelemetry environment variables (`OTEL_EXPORTER_OTLP_HEADERS`, `OTEL_EXPORTER_OTLP_CERTIFICATE`, ...) also apply to anything not set in the config file. Values in the config file take precedence. Environment variables are convenient in containers and Kubernetes.

Default ports: **4317** for gRPC and **4318** for HTTP.

### Authentication

Most hosted backends use a header:

```toml
[export.otlp.headers]
authorization = "Bearer YOUR_TOKEN"
```

The config file then holds a secret. Keep it readable only by you (`chmod 600 ~/.config/metricsd/config.toml`), or leave `headers` out and set `OTEL_EXPORTER_OTLP_HEADERS="authorization=Bearer YOUR_TOKEN"` in the environment instead, for example in a systemd drop-in or a Kubernetes secret.

### TLS

| Need | Settings |
| ---- | -------- |
| Public CA (most hosted services) | `https://` endpoint, nothing else |
| Private CA | `ca_file = "/etc/metricsd/ca.pem"` |
| Mutual TLS | `ca_file`, `cert_file`, and `key_file` |

Files are checked at startup; a missing file stops the daemon with the key name in the error. TLS 1.2 is the minimum.

## Data model

| metricsd | OTLP |
| -------- | ---- |
| Metric name (`mem.used_bytes`) | Metric name, unchanged |
| Every sample | A **Gauge** data point |
| Labels (`core=all,mode=user`) | Data point attributes (`core="all"`, `mode="user"`) |
| Name suffix | Unit, in UCUM: `By`, `By/s`, `%`, `1/s`, `ms`, `MHz`, `W`, `Cel`, `s`, `1/min`, `1` |
| Collection time | Data point timestamp: the moment it was measured, not the send time |

Rates (`*_per_sec`) and percentages are computed by metricsd before export, so every metric is a gauge with no cumulative counters to reset. See [metrics.md](metrics.md) for every name.

## Identity: resource attributes

Each export carries resource attributes describing **where metricsd runs**. A container on a VM on a server sends three independent identities; the server groups and links them.

| Attribute | Present | Example |
| --------- | ------- | ------- |
| `service.name` | always | `metricsd` |
| `service.version` | always | `v1.2.0` |
| `host.name` | always | `web-01`, or the container hostname |
| `host.arch` | always | `amd64` |
| `os.type` | always | `linux` |
| `os.description` | when known | `ubuntu 24.04`, `alpine 3.24.2` |
| `host.id` | physical machines and VMs | `/etc/machine-id` |
| `container.id` | Docker, Podman, other containers (when discoverable) | 64-hex container ID |
| `container.runtime.name` | Docker, Podman, other containers | `docker`, `podman` |
| `k8s.namespace.name`, `k8s.pod.name`, `k8s.pod.uid`, `k8s.container.name`, `k8s.node.name` | Kubernetes (each when known) | `prod`, `api-7d9f...` |
| `metricsd.environment` | always | `physical`, `vm`, `docker`, `podman`, `kubernetes`, `container` |
| `metricsd.id` | always | Stable unique ID of this source (same as `metricsctl info`) |
| `metricsd.hypervisor` | VMs | `kvm`, `vmware`, `aws`, ... |
| every `[tags]` entry | when configured | `env = "prod"` → `env="prod"`. Keys in the namespaces above (`service.`, `host.`, `os.`, `container.`, `k8s.`, `metricsd.`) are rejected at startup. |

```toml
[tags]
env    = "prod"
region = "us-east-1"
team   = "platform"
```

The same identity is available locally with `metricsctl info` and `GET /v1/info` ([api.md](api.md)).

## Delivery and buffering

The local SQLite database doubles as the export buffer.

- **Cursor.** metricsd records the timestamp of the last delivered cycle per exporter. Each tick it sends every newer cycle, in batches of up to 60 cycles, and advances the cursor only after the server accepts a batch. Nothing is sent twice, and nothing is skipped.
- **Outages.** If the server is unreachable, data stays in the database. Each request retries briefly (about 20 seconds); after that, metricsd waits and doubles the delay up to 5 minutes. When the server returns, the backlog is delivered with its original timestamps, as far back as `retention` keeps data. In testing, all 19 cycles collected while a collector was down arrived after it restarted.
- **Restarts.** The cursor survives metricsd restarts, so data from while metricsd itself was stopped is not lost either; there just isn't any, since nothing was collected.
- **New exporters** start at the present. They don't replay the whole retained history. Changing `protocol` counts as a new exporter.
- **Shutdown.** On SIGTERM metricsd makes one final delivery attempt (up to 5 seconds) before exiting.
- **Logging.** The first failure and every 10th consecutive failure are logged as warnings, so a long outage doesn't flood the journal. Recovery is logged once:

```
level=WARN msg="export failed; data is buffered locally" exporter=otlp-grpc err="... connection refused" failures=1 retry_in=1m0s
level=INFO msg="export recovered" exporter=otlp-grpc failures=2
```

A server that permanently rejects data (wrong token, wrong path) keeps failing until fixed; the backlog waits up to `retention`.

## Running under systemd

The unit allows `AF_INET` and `AF_INET6` in addition to `AF_UNIX` so export can reach the network. No other change is needed. See [systemd.md](systemd.md#sandboxing).

## Example configurations

Ready-to-copy versions with explanations: [examples/otlp-http/](../examples/otlp-http/) and [examples/otlp-grpc/](../examples/otlp-grpc/).

### OpenTelemetry Collector on the same network

```toml
[export.otlp]
enabled  = true
protocol = "grpc"
endpoint = "http://otel-collector.internal:4317"
interval = "15s"
```

### Grafana LGTM (`grafana/otel-lgtm`)

The all-in-one image listens on 4317 (gRPC) and 4318 (HTTP):

```toml
[export.otlp]
enabled  = true
protocol = "http"
endpoint = "http://localhost:4318"
```

Metrics appear in the bundled Prometheus/Mimir data source. OTLP-to-Prometheus conversion turns dots into underscores and appends units, so `mem.used_bytes` shows up as something like `mem_used_bytes`.

### Hosted vendor with a token

```toml
[tags]
env = "prod"

[export.otlp]
enabled  = true
protocol = "http"
endpoint = "https://otlp.vendor.example.com"
interval = "60s"

[export.otlp.headers]
authorization = "Bearer YOUR_TOKEN"
```

## Verify with a local collector

Run a throwaway collector that prints everything it receives. Map it to spare ports if 4317/4318 are already taken:

```yaml
# collector.yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318
exporters:
  debug:
    verbosity: detailed
service:
  pipelines:
    metrics:
      receivers: [otlp]
      exporters: [debug]
```

```bash
docker run -d --name otelcol -p 127.0.0.1:14317:4317 -p 127.0.0.1:14318:4318 \
  -v "$PWD/collector.yaml:/etc/otelcol/config.yaml:ro" otel/opentelemetry-collector:latest
```

Point metricsd at `http://127.0.0.1:14318` (http) or `http://127.0.0.1:14317` (grpc), wait one export interval, then:

```bash
docker logs otelcol 2>&1 | grep -E 'service.name|metricsd.environment|Name: cpu.usage_pct'
docker rm -f otelcol
```
