# Troubleshooting

Start with the logs — the daemon logs the exact reason it stopped:

```bash
systemctl --user status metricsd
journalctl --user -u metricsd -n 50 --no-pager
```

## metricsctl

### `cannot reach metricsd at ... (is it running?)`

The socket doesn't exist or nothing is listening.

1. Check the service: `systemctl --user status metricsd`.
2. If you set a custom `socket_path`, pass it: `metricsctl -socket /your/path now`.
3. When running `metricsd` by hand from another shell, check both shells have the same `XDG_RUNTIME_DIR`.

### `no samples collected yet`

The daemon just started. Wait one interval (5s by default).

### CPU, disk IO, or network metrics are missing

Rate metrics need two samples. They appear after the second cycle.

### `no data for metric "..." in the last 1h0m0s`

- Check the name with `metricsctl list` — names are exact (`cpu.usage_pct`, not `cpu`).
- The collector may be disabled under `[collectors]`.
- The range may predate your install; try a shorter `-since`.

### `step too small: range would exceed 10000 points`

`since / step` exceeds 10,000. Use a larger `-step` or omit it to get the automatic `since / 120`.

## Daemon won't start

### `load config: ... unknown keys: [...]`

A misspelled or unsupported key. Compare with [configuration.md](configuration.md#reference).

### `socket_path is N bytes; Unix sockets allow at most 107`

The kernel limits socket paths to 107 bytes. Choose a shorter path.

### `another metricsd is already running`

Another instance owns the socket. Find it:

```bash
systemctl --user status metricsd
pgrep -a metricsd
```

Stop the duplicate. A stale socket left by a crash is cleaned up automatically, so this message always means a live process.

### `... exists and is not a socket`

A regular file sits at `socket_path`. metricsd won't delete it; remove it yourself if it's safe.

### Service fails with `status=226/NAMESPACE`

A `ReadWritePaths=` entry in a drop-in points at a directory that doesn't exist. Create it, or prefix the entry with `-`. See [systemd.md](systemd.md#drop-ins).

### `create data dir: ... read-only file system` or `unable to open database file`

Under systemd, `db_path` points outside the writable sandbox. Either use the default location or add `ReadWritePaths=` for the directory with `systemctl --user edit metricsd`.

### Service keeps restarting

`Restart=on-failure` retries every 5s. The first error in `journalctl --user -u metricsd` is the cause; later entries repeat it.

## Unexpected data

### A mount point is missing

- Only filesystems on `/dev/...` devices are reported (no `tmpfs`, `overlay`, network filesystems).
- A device mounted several times (btrfs subvolumes) appears once, under its first mount in `/proc/mounts`.

### Disk IO shows the same numbers twice

An LVM or LUKS device (`dm-*`) mirrors the physical disk. It's excluded by default; if you changed `exclude_devices`, add `dm-*` back — or exclude the physical disk instead (see the [encrypted-storage example](../examples/encrypted-storage/)).

### Lots of `veth...` or `cali...` interfaces

Container networking creates one interface per container. Exclude them with `[net] exclude_interfaces`. The [docker-host](../examples/docker-host/) and [kubernetes-node](../examples/kubernetes-node/) examples have ready-made lists.

### No temperature sensors

- Virtual machines and many containers expose no hwmon devices — disable the `temp` collector.
- On bare metal, load the sensor driver (`sudo sensors-detect` from `lm_sensors` finds it) and restart metricsd.
- Check what the kernel exposes: `ls /sys/class/hwmon/*/temp*_input`.

### `disk.used_pct` differs from `df`

It shouldn't — both use `used / (used + available)`. If it differs, check you're comparing the same mount (`df /path`) and the same moment.

## Containers

See [containers.md](containers.md) for how container measurement works.

### A container reports the host's RAM and CPU count

- Check `metricsctl info`. `Environment` must be `docker`, `podman`, `kubernetes`, or `container`. If it says `physical` or `vm`, the runtime left no marker; set `environment = "docker"` (or the right runtime) in the config.
- The host runs cgroup v1. Container metrics need cgroup v2; the startup log says `cgroup v2 is not mounted`.
- The container has no limit. Without `--memory`/`--cpus`, the host's capacity is the real ceiling and is reported as such.

### `collector skipped` at startup

These are informational. Each line names the collector and the reason:

| Reason | Meaning |
| ------ | ------- |
| `machine-wide only; not measured inside containers` | load, temp, fan, battery are host figures. Run metricsd on the host for them. |
| `io controller not delegated to ...` | Rootless runtime without `io` delegation: no `diskio.*`. See [delegation](containers.md#controller-delegation). |
| `memory controller not delegated to ...` | No `mem.*` from the cgroup. |
| `no readable pressure files in ...` | The cgroup exposes no PSI files. |
| `pressure stall information unavailable ...` | The host kernel has PSI disabled (`psi=0`). |

### `create data dir: mkdir /var/lib/metricsd: permission denied`

Something set `db_path` to `/var/lib/...` while the container runs as a non-root user. Remove the setting (non-root users default to `/tmp/metricsd`) or mount a writable volume there.

### `read-only file system` in a container

The container runs with a read-only root filesystem. Add `--tmpfs /tmp` (Kubernetes: an `emptyDir` at `/tmp`) or point `db_path` and `socket_path` at a writable volume.

### `metricsctl` can't reach metricsd in a container

Run it inside the same container (`docker exec <name> metricsctl now`). The socket is inside the container's filesystem.

## Export

See [export.md](export.md).

### `export failed; data is buffered locally`

The server was unreachable or rejected the request; the `err` field says which. Data stays in the local database and is sent when delivery recovers. Only the first failure and every 10th are logged; `export recovered` marks the end.

### HTTP 404 from an OTLP/HTTP endpoint

The path is wrong. With an empty path metricsd posts to `/v1/metrics`; if your server expects another path, include it in `endpoint`. Also check that `protocol = "http"` is pointed at the HTTP port (usually 4318), not the gRPC port (4317).

### `Unavailable` / `connection refused` with gRPC

Check the port (4317) and the scheme: `http://` is plaintext, `https://` is TLS. A plaintext client against a TLS server, or the reverse, fails at connection time.

### `x509: certificate signed by unknown authority`

The server uses a private CA. Set `ca_file` to its PEM bundle.

### HTTP 401 / gRPC `Unauthenticated`

Missing or wrong credentials. Set `[export.otlp.headers]` (for example `authorization = "Bearer ..."`) or `OTEL_EXPORTER_OTLP_HEADERS`.

### Metrics arrive with old timestamps after an outage

Expected: that's the buffered backlog, sent with each sample's collection time.

### Ports 4317/4318 already in use when testing a local collector

Another collector (for example `grafana/otel-lgtm`) already owns them. Map the test collector to spare host ports (`-p 127.0.0.1:14317:4317`) and point `endpoint` there.

## Data and storage

### The database keeps growing

It grows until `retention` is reached, then stays roughly flat. See [architecture.md](architecture.md#disk-usage) for expected sizes. To shrink it after lowering `retention`, run `VACUUM` with the service stopped.

### Start fresh

```bash
systemctl --user stop metricsd
rm -f ~/.local/state/metricsd/metrics.db*
systemctl --user start metricsd
```

### Inspect the data directly

```bash
sqlite3 -readonly ~/.local/state/metricsd/metrics.db \
  "SELECT metric, count(*) FROM series GROUP BY metric;"
```
