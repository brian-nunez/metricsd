# Using metricsctl

`metricsctl` talks to a running `metricsd` over its Unix socket.

```
metricsctl [-socket PATH] <command> [flags]
```

| Global flag | Default | Description |
| ----------- | ------- | ----------- |
| `-socket`   | `$XDG_RUNTIME_DIR/metricsd/metricsd.sock`, else `/run/user/UID/...`, else `/tmp/metricsd/metricsd.sock` | Socket to connect to. Must match the daemon's `socket_path`. |

Global flags go **before** the command. Command flags can go before or after positional arguments.

Inside a container, run it with `docker exec` (or `podman exec`, `kubectl exec`): both programs pick the same default socket.

```bash
docker exec my-app metricsctl now
```

## now

Shows the latest value of every metric.

```console
$ metricsctl now
Collected 2026-10-17 21:59:09

METRIC                         LABELS                 VALUE
cpu.cores                                             12
cpu.usage_pct                  core=0                 1.0%
...
cpu.usage_pct                  core=all               1.6%
cpu.frequency_mhz              core=2                 1418 MHz
cpu.context_switches_per_sec                          6758.5/s
disk.used_pct                  mount=/                27.9%
diskio.read_latency_ms         device=zram0           0.02 ms
load.1m                                               0.62
mem.used_pct                                          55.4%
net.rx_bytes_per_sec           iface=enp3s0           860 B/s
pressure.some_pct              resource=io            0.4%
procs.count                                           433
tcp.established                                       19
temp.celsius                   chip=nvme,device=nvme0,sensor=Composite   38.9°C
uptime.seconds                                        6d 16h 8m
```

| Flag    | Description |
| ------- | ----------- |
| `-json` | Print the raw API response |

Rows are sorted naturally, so `core=2` comes before `core=10`. Values are formatted from the metric's unit:

| Unit (suffix) | Shown as |
| ------------- | -------- |
| `By` (`_bytes`) | `7.9 GiB` |
| `By/s` (`_bytes_per_sec`) | `860 B/s`, `2.4 MiB/s` |
| `%` (`_pct`) | `27.9%` |
| `1/s` (`_per_sec`) | `6758.5/s` |
| `MHz` (`_mhz`) | `1418 MHz` |
| `ms` (`_ms`) | `0.02 ms` |
| `W` (`_watts`) | `11.2 W` |
| `Cel` (`.celsius`) | `38.9°C` |
| `s` (`.seconds`) | `6d 16h 8m`, `3h 12m`, `4m` |
| `1/min` (`.rpm`) | `2650 RPM` |
| none | whole numbers as integers (`433`), others with two decimals (`0.62`) |

See [metrics.md](metrics.md#units) for what each metric means.

## list

Lists every metric currently being reported, with how many series (label combinations) each has.

```console
$ metricsctl list
METRIC                      SERIES
cpu.cores                   1
cpu.frequency_mhz           12
cpu.usage_pct               13
temp.celsius                15
...
```

| Flag    | Description |
| ------- | ----------- |
| `-json` | Print metric names as a JSON array |

## history

Shows one metric over time, averaged into fixed-width buckets.

```
metricsctl history METRIC [-since DURATION] [-step DURATION] [-labels LABELS] [-json]
```

| Flag      | Default        | Description |
| --------- | -------------- | ----------- |
| `-since`  | `1h`           | How far back to look. Go durations (`30m`, `12h`) or whole days (`7d`). Max `366d`. |
| `-step`   | `since / 120`  | Bucket width. Each bucket shows the average of the samples inside it. Minimum `1s`. |
| `-labels` | all series     | Show only the series whose labels match exactly, e.g. `core=all` or `mount=/`. |
| `-json`   | off            | Print JSON (filtered by `-labels` when given). |

Examples:

```bash
# Overall CPU for the last 6 hours, 15-minute averages
metricsctl history cpu.usage_pct -since 6h -step 15m -labels core=all

# Root filesystem usage over the past week, one point per day
metricsctl history disk.used_pct -since 7d -step 1d -labels mount=/

# Every temperature sensor for the last 30 minutes
metricsctl history temp.celsius -since 30m

# Was the container throttled by its CPU limit?
metricsctl history cpu.throttled_periods_pct -since 1h -step 1m

# Flags after the metric work too
metricsctl history mem.used_bytes --since 2h --step 10m
```

Output:

```console
$ metricsctl history cpu.usage_pct -since 5m -step 1m -labels core=all
cpu.usage_pct{core=all}
    19:00:00  2.1%
    19:01:00  1.5%
    19:02:00  1.0%
```

Timestamps show `HH:MM:SS` for ranges up to 24 hours and `Mon DD HH:MM` beyond that, in local time.

Buckets align to the Unix epoch, which means **UTC**. A `1h` step starts on the hour, but a `1d` step starts at 00:00 UTC, which displays as a non-midnight local time (for example `17:00` in UTC−7).

A range may contain at most 10,000 buckets per series. If `-since / -step` exceeds that, use a larger step.

## info

Shows the environment metricsd is measuring and the identity it reports to exporters.

```console
$ metricsctl info
Environment    physical
ID             b12f97a4-fe8a-4a2f-bacc-556caf2300a8
Hostname       bdesktop
OS             omarchy 4.0.1 (linux/amd64)
Kernel         7.1.9-arch1-2
CPU            12th Gen Intel(R) Core(TM) i5-12400F
Agent version  v0.1.0
Started        2026-10-17 21:59:03
```

Inside a Docker container:

```console
$ docker exec my-app metricsctl info
Environment        docker
ID                 b222ab9813a46ecdd4dbfbfa62d4e217a35058a277ef963b671e1431ca76b9f6
Hostname           b222ab9813a4
OS                 alpine 3.24.2 (linux/amd64)
Kernel             7.1.9-arch1-2
CPU                12th Gen Intel(R) Core(TM) i5-12400F
Container runtime  docker
Container ID       b222ab9813a46ecdd4dbfbfa62d4e217a35058a277ef963b671e1431ca76b9f6
Agent version      v0.1.0
Started            2026-10-18 04:59:40
```

VMs add a `Hypervisor` row, Kubernetes pods add `Namespace`, `Pod`, `Pod UID`, `Container`, and `Node`, and config tags appear as `Tag <key>` rows.

| Flag    | Description |
| ------- | ----------- |
| `-json` | Print the raw API response (see [api.md](api.md#get-v1info)) |

## health

Checks that the daemon is up and its database responds.

```console
$ metricsctl health
ok
```

Exits non-zero when the daemon is unreachable, which makes it handy in scripts:

```bash
metricsctl health >/dev/null || notify-send "metricsd is down"
```

## Exit codes

| Code | Meaning |
| ---- | ------- |
| 0    | Success |
| 1    | Request failed (daemon unreachable, unknown metric, bad parameter) |
| 2    | Usage error (unknown command, missing argument) |

## Scripting with JSON

Every command except `health` supports `-json`. Combine with `jq`:

```bash
# Current root filesystem usage as a bare number
metricsctl now -json | jq '.samples[] | select(.metric=="disk.used_pct" and .labels=="mount=/") | .value'

# Hottest sensor right now
metricsctl now -json | jq -r '[.samples[] | select(.metric=="temp.celsius")] | max_by(.value) | "\(.labels): \(.value)°C"'

# This source's ID
metricsctl info -json | jq -r .id
```

For direct HTTP access, see [api.md](api.md).

## metricsd version

```console
$ metricsd -version
v0.1.0
```

Builds from a git checkout print a pseudo-version such as `v0.0.0-20261018023215-99e4866c380e`; builds without version information print `dev`.
