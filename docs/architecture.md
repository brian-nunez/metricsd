# Architecture

## Data flow

```
                    ┌─ physical / vm ─► /proc, /sys collectors ─┐
 detect environment ┤                                           ├─► store (SQLite) ─┬─► API (Unix socket) ─► metricsctl / curl
                    └─ container ─────► cgroup v2 collectors ───┘        ▲          └─► export runner ─► OTLP (HTTP / gRPC)
                                                                   hourly prune
```

One process, one main loop:

1. **Startup**
   1. Load config.
   2. Resolve the environment (`environment = "auto"` detects it). See [containers.md](containers.md).
   3. Gather the identity (ID, hostname, OS, container or Kubernetes details, tags) served by `/v1/info` and sent with exports.
   4. Open the database and bind the socket.
   5. Pick each collector's implementation for the environment. Run checks, and skip collectors whose data source is absent.
   6. Run one collection (the baseline for rate metrics) and prune expired data.
   7. Start the export runner if an exporter is enabled.
2. **Every `interval`**: each collector reads its sources and returns samples. All samples from the cycle are written in one transaction under one shared timestamp.
3. **Every hour**: delete points older than `retention`, then delete series left with no points.
4. **Concurrently**:
   - The HTTP server answers API requests from a read-only connection pool.
   - The export runner sends new cycles to the configured endpoint.
5. **SIGTERM / SIGINT**:
   - Stop the loop.
   - The export runner makes a final delivery attempt (5s limit).
   - The API stops accepting requests (5s grace).
   - The database closes and the socket file is removed.

A collector that fails during a cycle logs a warning and is skipped for that cycle; the others still record.

## Environment and collector selection

| Collector | Physical / VM | Container |
| --------- | ------------- | --------- |
| `cpu` | `HostCPU` (gopsutil + `/proc/stat` + cpufreq) | `ContainerCPU` (`cpu.stat`, `cpu.max`) |
| `mem` | `HostMem` (gopsutil) | `ContainerMem` (`memory.*`) |
| `load` | `HostLoad` (gopsutil) | not collected |
| `disk` | `Disk` (block devices) | `Disk{Container: true}` (root overlay plus volumes) |
| `diskio` | `HostDiskIO` (gopsutil) | `ContainerDiskIO` (`io.stat`) |
| `net` | `Net` (gopsutil) | same, scoped by network namespace |
| `netstat` | `NetStat` (gopsutil + sockstat) | `NetStat{Container: true}` |
| `system` | `HostSystem` (gopsutil) | `ContainerSystem` (PID 1 age, `pids.*`) |
| `pressure` | `/proc/pressure/*` | cgroup `*.pressure` |
| `temp`, `fan`, `battery` | hwmon, power_supply | not collected |

Collectors that implement `Checker` are checked once at startup. A failed check disables the collector with one `collector skipped` log line, instead of logging the same failure every cycle. Container collectors use this for cgroup controllers that a rootless runtime didn't delegate. Hardware collectors on machines do **not** use it: they always run and rediscover hardware every cycle, so a battery or sensor that appears later is picked up.

### The collector interface

```go
type Collector interface {
	Name() string
	Collect(now time.Time) ([]Sample, error)
}

// Optional: checked once at startup.
type Checker interface {
	Check() error
}
```

Stateless collectors read and return. Counter-based collectors keep the previous readings in a `deltas` tracker: the first call records a baseline and returns no rates, and later calls emit `difference / elapsed seconds`.

### Container cgroup reader

Container metrics come from a small cgroup v2 reader (`internal/collector/cgroup.go`) instead of a library. It finds the process's cgroup from `/proc/self/cgroup`, resolving to the mount root under a private cgroup namespace (the default for Docker and Podman, rootful or rootless). It treats every interface file as optional.

## Packages

| Package | Responsibility |
| ------- | -------------- |
| `cmd/metricsd` | Wiring: config, signals, main loop, collector selection, export startup |
| `cmd/metricsctl` | CLI: commands, formatting, Unix socket HTTP client |
| `internal/environment` | Detects physical, VM, Docker, Podman, Kubernetes, or other container; standard library only |
| `internal/identity` | Builds the identity: machine ID or container ID, Kubernetes pod details, OS, tags |
| `internal/collector` | One collector per metric group and environment; cgroup v2 reader; `deltas` rate helper |
| `internal/store` | Schema, inserts, pruning, queries, export cursors |
| `internal/api` | HTTP handlers, parameter validation, socket listener |
| `internal/config` | Defaults, TOML loading, validation, environment-aware default paths |
| `internal/export` | `Exporter` interface and the runner that delivers buffered cycles |
| `internal/export/otlp` | OTLP/HTTP and OTLP/gRPC exporter; maps samples to OpenTelemetry gauges |
| `internal/version` | Version string from Go build info |
| `internal/wire` | JSON types and `Unit()`, the single source of metric units; no dependencies, so the CLI never links SQLite, gopsutil, or OpenTelemetry |

## Export

```go
type Exporter interface {
	Name() string
	Export(ctx context.Context, cycles []store.Cycle) error
	Shutdown(ctx context.Context) error
}
```

Samples stay in metricsd's own format (name, label string, value) all the way to the exporter, which translates them. New destinations are a new `Exporter`; collectors and storage don't change. Configuration and endpoint details are in [export.md](export.md).

### The store is the buffer

- Each exporter has a **cursor** in the `export_cursors` table: the timestamp of the last cycle it delivered.
- Every export `interval`, the runner reads cycles newer than the cursor, **60 cycles per request**, and sends them. It advances the cursor after each successful batch, so data that already arrived is never resent.
- It keeps sending until caught up, so a backlog drains in consecutive requests.
- On failure, it waits twice as long before each retry, up to **5 minutes**. It logs the first failure and every 10th after that, then logs once when delivery recovers.
- Data collected while the endpoint is down, or while metricsd is stopped, is delivered later with its original timestamps, as far back as `retention` keeps it.
- A **new exporter** (or a renamed one, such as after switching `protocol`) starts at the present instead of replaying all retained history.
- On shutdown, the runner makes a final delivery attempt before the database closes.

### OTLP mapping

| metricsd | OpenTelemetry |
| -------- | ------------- |
| metric name | metric name, unchanged |
| suffix | metric unit (UCUM, from `wire.Unit`) |
| value | `Gauge` data point (float64) |
| label pairs | data point attributes |
| cycle timestamp | data point time |
| identity (`/v1/info`) | resource attributes (`service.name=metricsd`, `host.*`, `os.*`, `container.*`, `k8s.*`, `metricsd.environment`, `metricsd.id`, tags) |

## Storage

### Engine

[`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite), a pure-Go translation of SQLite: no CGO, so the binary is static.

| Setting | Value | Why |
| ------- | ----- | --- |
| `journal_mode` | `WAL` | Readers never block the writer |
| `synchronous` | `NORMAL` | Safe with WAL; a power loss can drop at most the last cycle |
| `busy_timeout` | 5000 ms | Waits instead of failing under brief contention |
| Writer pool | 1 connection | SQLite has a single writer |
| Reader pool | 4 connections, `query_only` | API queries and exports run alongside collection |

### Schema

```sql
CREATE TABLE series (
	id     INTEGER PRIMARY KEY,
	metric TEXT NOT NULL,
	labels TEXT NOT NULL,
	UNIQUE (metric, labels)
);

CREATE TABLE points (
	series_id INTEGER NOT NULL REFERENCES series (id),
	ts        INTEGER NOT NULL,   -- Unix seconds
	value     REAL    NOT NULL,
	PRIMARY KEY (series_id, ts)
) WITHOUT ROWID;

CREATE INDEX points_ts ON points (ts);

CREATE TABLE export_cursors (
	name TEXT PRIMARY KEY,        -- exporter name, e.g. otlp-grpc
	ts   INTEGER NOT NULL         -- last delivered cycle
);
```

- **Normalized series.** Metric and label text is stored once per series; each point is just an id, a timestamp, and a value.
- **`WITHOUT ROWID` clustered on `(series_id, ts)`.** A history query reads one contiguous range per series.
- **`points_ts` index** serves pruning, the latest-cycle lookup, and export batches.
- Series ids are cached in memory after first use; the cache is cleared after each prune.

### Querying directly

The database is plain SQLite. Open it read-only while the daemon runs:

```bash
sqlite3 -readonly ~/.local/state/metricsd/metrics.db
```

```sql
-- Hourly average root filesystem usage for the last day
SELECT datetime((p.ts / 3600) * 3600, 'unixepoch', 'localtime') AS hour,
       round(avg(p.value), 1) AS used_pct
FROM points p JOIN series s ON s.id = p.series_id
WHERE s.metric = 'disk.used_pct' AND s.labels = 'mount=/'
  AND p.ts >= unixepoch() - 86400
GROUP BY hour ORDER BY hour;

-- Peak temperature per sensor this week
SELECT s.labels, max(p.value) AS peak_c
FROM points p JOIN series s ON s.id = p.series_id
WHERE s.metric = 'temp.celsius' AND p.ts >= unixepoch() - 7 * 86400
GROUP BY s.labels ORDER BY peak_c DESC;

-- How far behind each exporter is
SELECT name, (SELECT max(ts) FROM points) - ts AS seconds_behind FROM export_cursors;
```

### Disk usage

Measured at about **35 bytes per point** including indexes:

```
MiB per day ≈ series × (86400 / interval_seconds) × 35 / 1048576
```

**Physical machine, 169 series** (the measured desktop):

| Interval | Per day  | 1 day    | 7 days    | 30 days  | 90 days  |
| -------- | -------- | -------- | --------- | -------- | -------- |
| 1s       | 487 MiB  | 487 MiB  | 3.3 GiB   | 14.3 GiB | 42.8 GiB |
| 5s       | 97 MiB   | **97 MiB** (default) | 682 MiB | 2.9 GiB | 8.6 GiB |
| 15s      | 32 MiB   | 32 MiB   | 227 MiB   | 975 MiB  | 2.9 GiB  |
| 30s      | 16 MiB   | 16 MiB   | 114 MiB   | 487 MiB  | 1.4 GiB  |
| 60s      | 8.1 MiB  | 8.1 MiB  | 57 MiB    | 244 MiB  | 731 MiB  |

**Container, 73 series:**

| Interval | Per day  | 1 day    | 7 days    | 30 days  | 90 days  |
| -------- | -------- | -------- | --------- | -------- | -------- |
| 1s       | 211 MiB  | 211 MiB  | 1.4 GiB   | 6.2 GiB  | 18.5 GiB |
| 5s       | 42 MiB   | **42 MiB** (default) | 295 MiB | 1.2 GiB | 3.7 GiB |
| 15s      | 14 MiB   | 14 MiB   | 98 MiB    | 421 MiB  | 1.2 GiB  |
| 30s      | 7.0 MiB  | 7.0 MiB  | 49 MiB    | 210 MiB  | 632 MiB  |
| 60s      | 3.5 MiB  | 3.5 MiB  | 25 MiB    | 105 MiB  | 316 MiB  |

When metricsd mainly exports over OTLP, the local database only has to cover outages, and the default `retention` of `1d` keeps it small. Raise it when you want more local history or a longer outage buffer.

Pruning frees pages inside the file for reuse; the file itself doesn't shrink. After lowering retention, reclaim space once with:

```bash
systemctl --user stop metricsd
sqlite3 ~/.local/state/metricsd/metrics.db 'VACUUM;'
systemctl --user start metricsd
```

### Query performance

Measured on one day of 55 series at a 5s interval (950,000 points):

| Query | Time |
| ----- | ---- |
| Latest snapshot (`/v1/metrics`) | < 1 ms |
| 24 h history, one metric, 120 buckets | ~50 ms |

## Resource usage

| Resource | Typical |
| -------- | ------- |
| Memory (RSS) | 20–30 MB (measured 29 MB on a 12-thread desktop with OTLP export enabled) |
| CPU | well under 1% of one core at a 5s interval |
| Disk writes | one small transaction per interval |
| Network | one OTLP request per export interval when enabled (gzip by default) |

## Dependencies

| Module | Used for |
| ------ | -------- |
| `github.com/shirou/gopsutil/v4` | Machine-wide CPU, memory, load, disk, network, and process data |
| `modernc.org/sqlite` | Pure-Go SQLite (no CGO) |
| `github.com/BurntSushi/toml` | Config parsing with unknown-key detection |
| `golang.org/x/sys` | `statfs` and other syscalls |
| `go.opentelemetry.io/otel` SDK, `otlpmetrichttp`, `otlpmetricgrpc`, `google.golang.org/grpc` | OTLP export |

Container metrics deliberately don't use `github.com/containerd/cgroups`. Its `Stat()` fills missing controllers with zeros, so a rootless container without `cpu` or `io` delegation would report "0" instead of "unknown". It also logs errors every cycle on kernels without pressure stall information. The in-tree reader treats missing files as absent data.

## Design decisions

| Decision | Reason |
| -------- | ------ |
| Measure the current environment | A container on a VM on a server is three independent sources; the central server links them |
| Same metric names in every environment | Dashboards and alerts work unchanged on machines and containers |
| cgroup v2 files for containers | `/proc` inside a container describes the host; the cgroup holds the container's limits and usage |
| Every cgroup file optional | Rootless runtimes delegate only some controllers; missing data must not look like zero |
| Unix socket for the local API | No exposed port; filesystem permissions provide access control |
| Push via OTLP | Works from behind NAT and firewalls; vendor-neutral, accepted by most backends |
| Store-and-forward export | Outages and restarts don't lose data |
| SQLite instead of a TSDB | One file, zero ops, SQL for ad-hoc analysis; ample for one source |
| Pure-Go dependencies | Static binaries, `FROM scratch` image, simple cross-compilation |
| User service, not system service | Every metric is readable unprivileged; least privilege by default |
| Averaging on read | Full resolution is kept; any step can be requested later |
