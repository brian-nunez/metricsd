# HTTP API

metricsd serves a small read-only JSON API over HTTP on a **Unix socket**, not a TCP port.

| Property    | Value |
| ----------- | ----- |
| Socket      | `$XDG_RUNTIME_DIR/metricsd/metricsd.sock` (typically `/run/user/1000/metricsd/metricsd.sock`) |
| Permissions | `0600` – only your user can connect |
| Base path   | `/v1` |
| Format      | `application/json` |
| Host header | Ignored; any value works |

Because it is a socket, nothing on your network (including Tailscale or a VPN) can reach it, and browsers cannot open it.

## Connecting

### curl

```bash
SOCK="$XDG_RUNTIME_DIR/metricsd/metricsd.sock"
curl -s --unix-socket "$SOCK" http://x/v1/health
```

curl needs a hostname in the URL; `x` is a placeholder that is never resolved.

### Go

```go
client := &http.Client{
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sockPath)
		},
	},
}
resp, err := client.Get("http://metricsd/v1/metrics")
```

### Python

```python
import http.client, json, os, socket

class UnixHTTPConnection(http.client.HTTPConnection):
    def __init__(self, path):
        super().__init__("metricsd")
        self.path = path
    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.connect(self.path)

conn = UnixHTTPConnection(os.path.expandvars("$XDG_RUNTIME_DIR/metricsd/metricsd.sock"))
conn.request("GET", "/v1/metrics")
print(json.load(conn.getresponse()))
```

## Endpoints

### GET /v1/health

Checks that the daemon is running and the database responds.

```bash
curl -s --unix-socket "$SOCK" http://x/v1/health
```

```json
{"status": "ok"}
```

| Status | Meaning |
| ------ | ------- |
| 200    | Healthy |
| 503    | Database unavailable |

### GET /v1/info

The identity of the environment this metricsd measures. It is gathered once at startup. The same fields become the resource attributes on [OTLP exports](export.md), which is how a central server tells sources apart.

```bash
curl -s --unix-socket "$SOCK" http://x/v1/info
```

Physical machine:

```json
{
  "environment": "physical",
  "id": "b12f97a4-fe8a-4a2f-bacc-556caf2300a8",
  "hostname": "bdesktop",
  "os": "linux",
  "arch": "amd64",
  "platform": "omarchy",
  "platform_version": "4.0.1",
  "kernel_version": "7.1.9-arch1-2",
  "cpu_model": "12th Gen Intel(R) Core(TM) i5-12400F",
  "agent_version": "v0.1.0",
  "started_at": 1792299543
}
```

Docker container:

```json
{
  "environment": "docker",
  "id": "b222ab9813a46ecdd4dbfbfa62d4e217a35058a277ef963b671e1431ca76b9f6",
  "hostname": "b222ab9813a4",
  "os": "linux",
  "arch": "amd64",
  "platform": "alpine",
  "platform_version": "3.24.2",
  "kernel_version": "7.1.9-arch1-2",
  "cpu_model": "12th Gen Intel(R) Core(TM) i5-12400F",
  "container": {
    "runtime": "docker",
    "id": "b222ab9813a46ecdd4dbfbfa62d4e217a35058a277ef963b671e1431ca76b9f6"
  },
  "tags": {"service_group": "api"},
  "agent_version": "v0.1.0",
  "started_at": 1792299580
}
```

| Field | Type | Description |
| ----- | ---- | ----------- |
| `environment` | string | `physical`, `vm`, `docker`, `podman`, `kubernetes`, or `container` |
| `id` | string | Stable unique ID: `/etc/machine-id` on machines, the container ID in containers, `<pod-uid>/<container>` in Kubernetes. Falls back to the hostname. |
| `hostname` | string | Hostname (the short container ID in Docker by default, the pod name in Kubernetes) |
| `os`, `arch` | string | Go platform, e.g. `linux`, `amd64` |
| `platform`, `platform_version` | string | Distribution from `/etc/os-release`; inside a container, the container image's OS |
| `kernel_version` | string | Running kernel (a container shares its host's kernel) |
| `cpu_model` | string | CPU model name |
| `hypervisor` | string | VMs only: `kvm`, `vmware`, `hyperv`, `xen`, `aws`, `gce`, ... or `unknown` |
| `container.runtime` | string | `docker`, `podman`, or `container`; empty in Kubernetes |
| `container.id` | string | 64-character container ID when discoverable |
| `kubernetes.namespace`, `.pod`, `.pod_uid`, `.container`, `.node` | string | Kubernetes only; `.node` needs `NODE_NAME` from the downward API |
| `tags` | object | `[tags]` from the config |
| `agent_version` | string | metricsd version |
| `started_at` | int | Unix time the daemon started |

Empty fields are omitted.

### GET /v1/metrics

Every sample from the most recent collection cycle.

```bash
curl -s --unix-socket "$SOCK" http://x/v1/metrics
```

```json
{
  "ts": 1792292599,
  "samples": [
    {"metric": "cpu.usage_pct", "labels": "core=all", "value": 1.04},
    {"metric": "disk.used_pct", "labels": "mount=/", "value": 27.89},
    {"metric": "load.1m", "value": 0.14},
    {"metric": "mem.used_bytes", "value": 8261513216}
  ]
}
```

| Field               | Type   | Description |
| ------------------- | ------ | ----------- |
| `ts`                | int    | Unix time of the cycle (seconds). `0` if nothing has been collected yet. |
| `samples[].metric`  | string | Metric name |
| `samples[].labels`  | string | Label string; omitted when empty |
| `samples[].value`   | number | Raw value in the metric's base unit (see [metrics.md](metrics.md#units)) |

Samples are ordered by metric, then labels (byte order).

### GET /v1/metrics/{name}

One metric's history, averaged into fixed-width time buckets.

| Query parameter | Default        | Limits | Description |
| --------------- | -------------- | ------ | ----------- |
| `since`         | `1h`           | `1s` – `366d` | How far back from now. Go duration or days (`7d`). |
| `step`          | `since / 120` (min `1s`) | ≥ `1s`; at most 10,000 buckets | Bucket width; truncated to whole seconds. |

```bash
curl -s --unix-socket "$SOCK" "http://x/v1/metrics/cpu.usage_pct?since=30m&step=10m"
```

```json
{
  "metric": "cpu.usage_pct",
  "since": 1792290799,
  "step_seconds": 600,
  "series": [
    {
      "labels": "core=0",
      "points": [
        {"ts": 1792290600, "value": 2.31},
        {"ts": 1792291200, "value": 1.87},
        {"ts": 1792291800, "value": 1.02}
      ]
    },
    {
      "labels": "core=all",
      "points": [
        {"ts": 1792290600, "value": 2.12},
        {"ts": 1792291200, "value": 1.64},
        {"ts": 1792291800, "value": 0.98}
      ]
    }
  ]
}
```

| Field                     | Type   | Description |
| ------------------------- | ------ | ----------- |
| `metric`                  | string | Requested metric |
| `since`                   | int    | Start of the range (Unix seconds) |
| `step_seconds`            | int    | Bucket width actually used |
| `series[].labels`         | string | Label string; omitted when empty |
| `series[].points[].ts`    | int    | Bucket start, aligned to a multiple of `step_seconds` since the Unix epoch (so day buckets start at 00:00 UTC) |
| `series[].points[].value` | number | Average of samples in the bucket |

- Buckets with no samples are omitted, not zero-filled. Gaps mean the daemon wasn't running.
- The first bucket may start before `since` because of alignment; it averages only samples at or after `since`.
- Series are ordered by labels (byte order); points by time.

| Status | Meaning |
| ------ | ------- |
| 200    | OK |
| 400    | Invalid `since` or `step`, or too many buckets |
| 404    | Unknown metric, or no data in the range |

## Errors

Errors from the endpoints above return JSON:

```json
{"error": "step too small: range would exceed 10000 points"}
```

| Status | When | Body |
| ------ | ---- | ---- |
| 400 | Bad query parameter | JSON |
| 404 | No data for the metric in the range | JSON |
| 500 | Database query failed (details in the daemon log) | JSON |
| 503 | Health check failed | JSON |
| 404 | Unknown path (e.g. `/v2/...`) | plain text `404 page not found` |
| 405 | Method other than `GET` | plain text `Method Not Allowed` |

Check the status code first; parse the body as JSON only for the first four.

## Limits and timeouts

| Limit                      | Value |
| -------------------------- | ----- |
| Query timeout              | 20s |
| Response write timeout     | 30s |
| Max buckets per series     | 10,000 |
| Max `since`                | 366 days |

## Stability

Endpoints live under `/v1`. Within `v1`, fields may be added but existing fields will not change meaning or be removed.
