# Architecture

## Data flow

```
 /proc, /sys ──► collectors ──► store (SQLite) ──► API (Unix socket) ──► metricsctl / curl
                    ▲                ▲
              every interval    hourly prune
```

One process, one main loop:

1. **Startup** – load config, open the database, bind the socket, run one collection (the baseline for rate metrics), prune expired data.
2. **Every `interval`** – each enabled collector reads its kernel files and returns samples; all samples from the cycle are written in one transaction with one shared timestamp.
3. **Every hour** – delete points older than `retention`, then delete series with no remaining points.
4. **Concurrently** – the HTTP server answers API requests from a read-only connection pool.
5. **SIGTERM / SIGINT** – stop accepting requests (5s grace), stop the loop, close the database. The socket file is removed.

A failing collector logs a warning and is skipped for that cycle; the others still record.

## Packages

| Package | Responsibility |
| ------- | -------------- |
| `cmd/metricsd` | Wiring: config, signals, main loop, collector registry |
| `cmd/metricsctl` | CLI: commands, formatting, Unix socket HTTP client |
| `internal/collector` | One collector per metric group behind a `Collector` interface; shared counter-to-rate helper |
| `internal/store` | Schema, inserts, pruning, latest and history queries |
| `internal/api` | HTTP handlers, parameter validation, socket listener |
| `internal/config` | Defaults, TOML loading, validation, XDG paths, duration parsing |
| `internal/wire` | JSON request/response types; no dependencies, so the CLI never links SQLite |

### The collector interface

```go
type Collector interface {
	Name() string
	Collect(now time.Time) ([]Sample, error)
}
```

Stateless collectors (memory, load, disk, temperature) read and return. Stateful collectors (CPU, disk IO, network) keep the previous counter reading, return nothing on the first call, and compute deltas afterwards.

## Storage

### Engine

[`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite), a pure-Go translation of SQLite: no CGO, so the binary is static.

| Setting | Value | Why |
| ------- | ----- | --- |
| `journal_mode` | `WAL` | Readers never block the writer |
| `synchronous` | `NORMAL` | Safe with WAL; a power loss can drop at most the last cycle |
| `busy_timeout` | 5000 ms | Waits instead of failing under brief contention |
| Writer pool | 1 connection | SQLite has a single writer |
| Reader pool | 4 connections, `query_only` | API queries run alongside collection |

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
```

- **Normalized series.** Metric and label text is stored once per series; each point is just an id, a timestamp, and a value.
- **`WITHOUT ROWID` clustered on `(series_id, ts)`.** A history query reads one contiguous range per series.
- **`points_ts` index** serves pruning and the "latest cycle" lookup.
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
```

### Disk usage

Measured at about **35 bytes per point** including indexes:

```
MiB per day ≈ series × (86400 / interval_seconds) × 35 / 1048576
```

For a typical desktop with **55 series**:

| Interval | Per day   | 1 day retention | 7 days    | 30 days   | 90 days  |
| -------- | --------- | --------------- | --------- | --------- | -------- |
| 1s       | 159 MiB   | 159 MiB         | 1.1 GiB   | –         | –        |
| 5s       | 32 MiB    | 32 MiB          | **222 MiB** (default) | 952 MiB | 2.8 GiB |
| 15s      | 11 MiB    | 11 MiB          | 74 MiB    | 317 MiB   | 952 MiB  |
| 30s      | 5.3 MiB   | 5.3 MiB         | 37 MiB    | 159 MiB   | 476 MiB  |
| 60s      | 2.6 MiB   | 2.6 MiB         | 19 MiB    | 79 MiB    | 238 MiB  |

Pruning frees pages inside the file for reuse; the file itself doesn't shrink. After lowering retention, reclaim space once with:

```bash
systemctl --user stop metricsd
sqlite3 ~/.local/state/metricsd/metrics.db 'VACUUM;'
systemctl --user start metricsd
```

### Query performance

Measured on one day of data at the default settings (950,000 points):

| Query | Time |
| ----- | ---- |
| Latest snapshot (`/v1/metrics`) | < 1 ms |
| 24 h history, one metric, 120 buckets | ~50 ms |

## Resource usage

| Resource | Typical |
| -------- | ------- |
| Memory (RSS) | 15–25 MB |
| CPU | well under 1% of one core at a 5s interval |
| Disk writes | one small transaction per interval |

## Design decisions

| Decision | Reason |
| -------- | ------ |
| Read `/proc` and `/sys` directly | No dependencies, no root, exact control over what's measured |
| Unix socket instead of TCP | No exposed port; filesystem permissions provide access control |
| SQLite instead of a TSDB | One file, zero ops, SQL for ad-hoc analysis; ample for one host |
| Pure-Go SQLite driver | Static binary, simple cross-compilation |
| User service, not system service | Every metric is readable unprivileged; least privilege by default |
| Averaging on read | Full resolution is kept; any step can be requested later |
