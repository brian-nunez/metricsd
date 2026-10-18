# Using metricsctl

`metricsctl` talks to a running `metricsd` over its Unix socket.

```
metricsctl [-socket PATH] <command> [flags]
```

| Global flag | Default | Description |
| ----------- | ------- | ----------- |
| `-socket`   | `$XDG_RUNTIME_DIR/metricsd/metricsd.sock` | Socket to connect to. Must match the daemon's `socket_path`. |

Global flags go **before** the command. Command flags can go before or after positional arguments.

## now

Shows the latest value of every metric.

```console
$ metricsctl now
Collected 2026-10-17 19:03:19

METRIC                      LABELS          VALUE
cpu.usage_pct               core=0          1.0%
cpu.usage_pct               core=1          1.0%
...
cpu.usage_pct               core=all        1.0%
disk.free_bytes             mount=/         669.8 GiB
disk.used_pct               mount=/         27.9%
load.1m                                     0.14
mem.used_bytes                              7.9 GiB
net.rx_bytes_per_sec        iface=enp3s0    616 B/s
temp.celsius                chip=nvme,device=nvme0,sensor=Composite   38.9°C
uptime.seconds                              6d 16h 8m
```

| Flag    | Description |
| ------- | ----------- |
| `-json` | Print the raw API response |

Rows are sorted naturally, so `core=2` comes before `core=10`. Values are formatted from the metric's unit suffix (see [metrics.md](metrics.md#units)).

## list

Lists every metric currently being reported, with how many series (label combinations) each has.

```console
$ metricsctl list
METRIC                      SERIES
cpu.usage_pct               13
disk.free_bytes             2
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
```

For direct HTTP access, see [api.md](api.md).
