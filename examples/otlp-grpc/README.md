# otlp-grpc

Exports metrics over **OTLP/gRPC** to a self-hosted OpenTelemetry Collector, secured with TLS from a private CA and, optionally, mutual TLS.

## Settings

| Key | Value | Why |
| --- | ----- | --- |
| `[tags] env` | `production` | Sent as a resource attribute with every export. |
| `protocol` | `grpc` | OTLP/gRPC, default port 4317. Efficient long-lived connections; the usual choice between collectors. |
| `endpoint` | `https://otel-collector.internal.example.com:4317` | `https://` enables TLS. For gRPC the path is ignored. |
| `interval` | `30s` | Buffered cycles are sent every 30 seconds. |
| `ca_file` | commented out | Uncomment when the collector's certificate is signed by your own CA. |
| `cert_file`, `key_file` | commented out | Uncomment for mutual TLS. |

Certificate lines are commented out because metricsd validates that each configured file exists at startup. A public certificate (Let's Encrypt, etc.) needs no `ca_file`.

## Private CA

```bash
sudo install -d -m 755 /etc/metricsd
sudo install -m 644 ca.pem /etc/metricsd/ca.pem
```

Then uncomment `ca_file`. The CA is added to the system roots, so public endpoints keep working too.

## Mutual TLS

```bash
sudo install -m 644 client.pem     /etc/metricsd/client.pem
sudo install -m 600 -o "$USER" client-key.pem /etc/metricsd/client-key.pem
```

Uncomment `cert_file` and `key_file`. The key must be readable by the user metricsd runs as. The collector side:

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
        tls:
          cert_file: /etc/otelcol/server.pem
          key_file: /etc/otelcol/server-key.pem
          client_ca_file: /etc/otelcol/ca.pem   # require client certificates
```

## Local collector without TLS

For a collector on the same machine or a trusted private network (a sidecar, a Docker Compose service, Grafana's `otel-lgtm` image), use plaintext:

```toml
[export.otlp]
enabled  = true
protocol = "grpc"
endpoint = "http://127.0.0.1:4317"
```

`http://` disables TLS. Never use it across untrusted networks.

## gRPC or HTTP?

| | gRPC | HTTP |
| --- | --- | --- |
| Default port | 4317 | 4318 |
| Through HTTP proxies and many load balancers | needs HTTP/2 end to end | works everywhere |
| Typical use | collector to collector, inside a cluster | hosted backends, the public internet |

Both carry identical data. See [otlp-http](../otlp-http/) for the HTTP variant.

More: [docs/export.md](../../docs/export.md).
