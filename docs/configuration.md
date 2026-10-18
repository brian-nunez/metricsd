# Configuration

metricsd works without a config file. To customize it, create a [TOML](https://toml.io) file at:

```
~/.config/metricsd/config.toml        # or $XDG_CONFIG_HOME/metricsd/config.toml
```

A fully commented reference with every default is in [`deploy/config.example.toml`](../deploy/config.example.toml). Ready-made configs for common setups are in [`examples/`](../examples/README.md).

Apply changes by restarting:

```bash
systemctl --user restart metricsd
```

## How the file is loaded

- **Defaults first.** Every key is optional; anything you leave out keeps its default.
- **Lists replace, maps merge.** Setting `exclude_interfaces` replaces the whole default list, so include the defaults you still want. Setting one entry under `[collectors]` changes only that collector.
- **Unknown keys are errors.** A typo such as `intreval` stops the daemon with `unknown keys: [intreval]` instead of being silently ignored.
- **Missing file is fine** at the default path. A missing file passed with `-config` is an error.

```bash
metricsd -config /path/to/config.toml
```

## Reference

### Top-level keys

| Key           | Type     | Default | Description |
| ------------- | -------- | ------- | ----------- |
| `environment` | string   | `"auto"` | What metricsd is measuring. See [Environment](#environment). |
| `interval`    | duration | `"5s"`  | Time between samples. Minimum `1s`. |
| `retention`   | duration | `"7d"`  | How long samples are kept. Must be at least `interval`. Old samples are pruned at startup and then hourly. Also bounds how far back [export](#exportotlp) can catch up after an outage. |
| `db_path`     | path     | [depends on environment](#default-paths) | SQLite database file. Parent directory is created if needed (mode `0700`). |
| `socket_path` | path     | [see below](#default-paths) | Unix socket for the API. At most 107 bytes. |

### Environment

metricsd measures the environment it runs in. A container on a VM on a server, each running metricsd, gives three independent sources, each reporting its own CPU, memory, and disks.

| Value        | Meaning | Collected from |
| ------------ | ------- | -------------- |
| `auto`       | Detect at startup (default) | – |
| `physical`   | Bare-metal machine, PC, laptop | `/proc`, `/sys` |
| `vm`         | Virtual machine (KVM, VMware, Hyper-V, cloud instances, …) | `/proc`, `/sys` |
| `docker`     | Docker container, rootful or rootless | the container's cgroup v2 |
| `podman`     | Podman container, rootful or rootless | the container's cgroup v2 |
| `kubernetes` | Container in a Kubernetes pod | the container's cgroup v2 |
| `container`  | Any other runtime that sets `container=` (LXC, systemd-nspawn) | the container's cgroup v2 |

Detection with `auto`:

1. `KUBERNETES_SERVICE_HOST` set, or `/var/run/secrets/kubernetes.io` exists → `kubernetes`
2. `/run/.containerenv` exists, or `container=podman` → `podman`
3. `/.dockerenv` exists → `docker`
4. `container` environment variable set → `container`
5. CPU `hypervisor` flag (x86) or a virtual DMI vendor (other architectures) → `vm`
6. Otherwise → `physical`

Set it explicitly only when detection is wrong, for example a container runtime that leaves no marker. The value also selects the default database path. The detected environment is logged at startup and shown by `metricsctl info`.

Container environments require cgroup v2 (the default on every current distribution). On cgroup v1 hosts, the CPU, memory, disk IO, and pressure collectors are skipped with a log message. See [containers.md](containers.md).

### `[tags]`

Free-form key/value pairs that describe this source. They appear in `metricsctl info` and `/v1/info`, and are sent with every [OTLP export](export.md) as resource attributes, so the central server can group sources.

```toml
[tags]
env    = "production"
region = "us-east-1"
team   = "platform"
```

Keys must not be empty. Values are strings. Keys starting with `service.`, `host.`, `os.`, `container.`, `k8s.`, or `metricsd.` are reserved for the identity metricsd detects and are rejected, so a tag can never overwrite the real hostname or container ID in exports.

### `[collectors]`

Each collector can be switched on or off. All are on by default. Some exist only on machines; inside containers they are skipped with a log line, whatever this table says.

| Key        | Metrics produced | Machines (`physical`, `vm`) | Containers |
| ---------- | ---------------- | :---: | :---: |
| `cpu`      | `cpu.*` usage, modes, cores, frequency, interrupts; throttling in containers | ✓ | ✓ |
| `mem`      | `mem.*`, `swap.*` | ✓ | ✓ |
| `load`     | `load.1m`, `load.5m`, `load.15m` | ✓ | – |
| `disk`     | `disk.*` space and inodes | ✓ | ✓ (root filesystem and volumes) |
| `diskio`   | `diskio.*` throughput, IOPS, latency, utilization | ✓ | ✓ (throughput, IOPS) |
| `net`      | `net.*` bytes, packets, errors, drops per interface | ✓ | ✓ |
| `netstat`  | `tcp.*`, `udp.*`, `sockets.used` | ✓ | ✓ |
| `system`   | `uptime.seconds`, `procs.*`, `fs.*`, `users.logged_in` | ✓ | ✓ (uptime, processes, task limit) |
| `pressure` | `pressure.some_pct`, `pressure.full_pct` | ✓ | ✓ |
| `temp`     | `temp.celsius` | ✓ | – |
| `fan`      | `fan.rpm` | ✓ | – |
| `battery`  | `battery.*`, `power.ac_online` | ✓ | – |

```toml
[collectors]
netstat = false
```

- Disabling a collector stops new samples. Its existing history stays queryable until retention removes it.
- `temp`, `fan`, and `battery` look for hardware every cycle. A machine without that hardware reports nothing, and hardware that appears later (a docked laptop, a hot-plugged UPS, a late-loading sensor driver) is picked up automatically, so leaving them on everywhere is safe.
- In containers, collectors whose cgroup files are missing are skipped at startup with the reason logged. This is normal for rootless runtimes without full cgroup delegation; see [containers.md](containers.md).

Full metric list: [metrics.md](metrics.md).

### `[net]`

| Key                  | Type          | Default                                | Description |
| -------------------- | ------------- | -------------------------------------- | ----------- |
| `exclude_interfaces` | list of globs | `["lo", "veth*", "br-*", "docker*"]`   | Interfaces to skip. Names come from `/proc/net/dev` (or `ip -br link`). |

### `[diskio]`

| Key               | Type          | Default                       | Description |
| ----------------- | ------------- | ----------------------------- | ----------- |
| `exclude_devices` | list of globs | `["loop*", "ram*", "dm-*"]`   | Block devices to skip. Only whole devices are ever reported; partitions are always skipped. |

`dm-*` is excluded by default because device-mapper devices (LUKS, LVM) duplicate the I/O of the disk underneath them. See the [encrypted-storage example](../examples/encrypted-storage/) to track them instead.

### `[export.otlp]`

Sends every collected cycle to an OpenTelemetry endpoint over OTLP/HTTP or OTLP/gRPC. Off by default. Delivery, buffering, and resource attributes are explained in [export.md](export.md).

| Key           | Type     | Default  | Description |
| ------------- | -------- | -------- | ----------- |
| `enabled`     | bool     | `false`  | Turn export on. The other keys are validated only when this is `true`. |
| `protocol`    | string   | `"http"` | `"http"` (OTLP/HTTP protobuf, port 4318) or `"grpc"` (port 4317). |
| `endpoint`    | URL      | empty    | Collector URL. See rules below. |
| `headers`     | table    | none     | Extra request headers (HTTP) or metadata (gRPC), typically authentication. |
| `compression` | string   | `"gzip"` | `"gzip"` or `"none"`. |
| `timeout`     | duration | `"10s"`  | Per-request timeout. |
| `interval`    | duration | `"30s"`  | How often buffered cycles are sent. Minimum `1s`. |
| `ca_file`     | path     | none     | PEM CA bundle to trust in addition to the system roots (private CA). |
| `cert_file`   | path     | none     | Client certificate for mutual TLS. Requires `key_file`. |
| `key_file`    | path     | none     | Client private key for mutual TLS. Requires `cert_file`. |

Endpoint rules:

- **Scheme picks TLS.** `https://` uses TLS; `http://` sends plaintext (local collectors, sidecars).
- **HTTP path.** With `protocol = "http"`, a URL with no path (or `/`) gets `/v1/metrics` appended. A URL with any other path is used exactly as written, for backends with custom paths.
- **gRPC path** is ignored; only scheme, host, and port matter.
- **Empty endpoint** defers to the standard `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` / `OTEL_EXPORTER_OTLP_ENDPOINT` environment variables, and then to `localhost:4318` (HTTP) or `localhost:4317` (gRPC). Handy in containers and Kubernetes, where the endpoint comes from the environment.

```toml
[export.otlp]
enabled  = true
protocol = "http"
endpoint = "https://otlp.example.com"   # posts to https://otlp.example.com/v1/metrics

[export.otlp.headers]
authorization = "Bearer REPLACE_ME"
```

The config file now holds a credential: keep it readable only by you (`chmod 600`). Headers set here take precedence over `OTEL_EXPORTER_OTLP_HEADERS`.

Ready-made: [otlp-http](../examples/otlp-http/), [otlp-grpc](../examples/otlp-grpc/).

## Default paths

Defaults depend on the environment so the daemon works out of the box everywhere, including containers with no home directory and rootless runtimes.

### Database (`db_path`)

| Situation | Default |
| --------- | ------- |
| systemd set `$STATE_DIRECTORY` (the shipped unit) | `$STATE_DIRECTORY/metrics.db` (first entry if several, colon-separated) |
| Machine (`physical`, `vm`) | `$XDG_STATE_HOME/metricsd/metrics.db`, normally `~/.local/state/metricsd/metrics.db` |
| Container, running as root (UID 0 inside the container, including rootless runtimes) | `/var/lib/metricsd/metrics.db` |
| Container, running as a non-root user | `/tmp/metricsd/metrics.db` |

Container storage is usually ephemeral; mount a volume at the database directory if history must survive container replacement. With [export](export.md) enabled, the local database is only a buffer and ephemeral storage is fine.

### Socket (`socket_path`)

The same rule applies to metricsd and metricsctl, so they always agree:

| Situation | Default |
| --------- | ------- |
| `$XDG_RUNTIME_DIR` is set | `$XDG_RUNTIME_DIR/metricsd/metricsd.sock` |
| `/run/user/<UID>` exists | `/run/user/<UID>/metricsd/metricsd.sock` |
| Otherwise (containers, minimal systems) | `/tmp/metricsd/metricsd.sock` |

## Value formats

### Durations

| Form            | Examples                      |
| --------------- | ----------------------------- |
| Go duration     | `"500ms"`, `"30s"`, `"5m"`, `"12h"`, `"1h30m"` |
| Whole/fractional days | `"7d"`, `"90d"`, `"1.5d"` |

Mixed day forms such as `"1d12h"` are not supported; write `"36h"`.

### Paths

- Must be absolute.
- A leading `~/` expands to your home directory.
- Environment variables are **not** expanded (`$HOME/x` is rejected as relative).

### Globs

Shell-style patterns matched against the whole name:

| Pattern  | Matches |
| -------- | ------- |
| `*`      | any run of characters |
| `?`      | one character |
| `[abc]`, `[a-z]` | one character from the set |
| `\*`     | a literal `*` |

`veth*` matches `veth2649d83`; `wl*` matches every Wi-Fi interface; `nvme?n1` matches `nvme0n1` and `nvme1n1`. Invalid patterns (for example an unclosed `[`) are rejected at startup.

## Validation rules

The daemon refuses to start, logging the reason, when:

| Rule | Example error |
| ---- | ------------- |
| Unknown environment | `invalid environment "lxc" (valid: auto, physical, vm, docker, podman, kubernetes, container)` |
| `interval` < 1s | `interval must be at least 1s, got 500ms` |
| `retention` < `interval` | `retention (1s) must be at least the interval (5s)` |
| Relative or empty path | `db_path must be an absolute path, got "data/metrics.db"` |
| Socket path > 107 bytes | `socket_path is 120 bytes; Unix sockets allow at most 107` |
| Empty tag key | `tags: keys must not be empty` |
| Reserved tag key | `tags: "host.name" is reserved (keys may not start with service., host., os., container., k8s., metricsd.)` |
| Unknown collector | `unknown collector "gpu" (valid: cpu, mem, load, disk, diskio, net, netstat, system, pressure, temp, fan, battery)` |
| Bad glob | `invalid exclude pattern "[eth": syntax error in pattern` |
| Unknown key | `unknown keys: [intreval]` |
| Bad OTLP protocol | `export.otlp: protocol must be "http" or "grpc", got "udp"` |
| Bad OTLP endpoint | `export.otlp: endpoint must be a URL like https://collector.example.com:4318, got "collector:4318"` |
| Bad compression | `export.otlp: compression must be "gzip" or "none", got "zstd"` |
| Non-positive timeout | `export.otlp: timeout must be positive` |
| Export interval < 1s | `export.otlp: interval must be at least 1s, got 500ms` |
| Half a client certificate | `export.otlp: cert_file and key_file must be set together` |
| Missing certificate file | `export.otlp: ca_file: stat /etc/metricsd/ca.pem: no such file or directory` |

`export.otlp` keys are checked only when `enabled = true`.

Check the reason with `journalctl --user -u metricsd -n 20` (or `docker logs` in a container).

## Changing paths under systemd

The service's sandbox only allows writes to `~/.local/state/metricsd` and `$XDG_RUNTIME_DIR/metricsd`. A custom `db_path` or `socket_path` elsewhere needs a drop-in granting write access. The [custom-paths example](../examples/custom-paths/) walks through it, and [systemd.md](systemd.md#drop-ins) explains drop-ins.

If you change `socket_path`, pass the same path to `metricsctl -socket`.

## Choosing interval and retention

Disk usage grows with series × samples kept. At about 35 bytes per point:

```
MiB per day ≈ series × (86400 / interval_seconds) × 35 / 1048576
```

A typical desktop reports about **169 series**; a container about **73**. At the default 5s interval that is roughly 98 MiB/day for a desktop and 42 MiB/day for a container. Use `metricsctl list` to see your own count, and see [architecture.md](architecture.md#disk-usage) for a sizing table.
