# Metrics reference

Each value metricsd stores is a **sample**: a metric name, a label string, and a number. A metric plus one specific label string is a **series** (for example `cpu.usage_pct` with `core=3`).

## Naming

- Names are `group.name_unit`, for example `mem.used_bytes`.
- Labels are comma-separated `key=value` pairs, for example `chip=nvme,device=nvme0,sensor=Composite`.
- Metrics with a single series have an empty label string.

## Units

`metricsctl` formats values from the name's suffix. The API always returns raw numbers.

| Suffix            | Raw unit       | Displayed as |
| ----------------- | -------------- | ------------ |
| `_bytes`          | bytes          | `7.9 GiB` (binary prefixes) |
| `_bytes_per_sec`  | bytes/second   | `616 B/s`, `17.1 MiB/s` |
| `_pct`            | percent 0–100  | `27.9%` |
| `.celsius`        | degrees C      | `38.9°C` |
| `.seconds`        | seconds        | `6d 16h 8m` |
| none (`load.*`)   | plain number   | `0.14` |

## Gauges and rates

- **Gauges** (memory, disk space, load, temperature, uptime) are read as-is each cycle.
- **Rates** (CPU usage, disk IO, network) come from counters that only increase. metricsd takes the difference between two consecutive reads and divides by elapsed time. Rates therefore appear from the **second** cycle on, and a counter that resets (device removed and re-added) is skipped for one cycle.

## CPU – `cpu` collector

| Metric          | Labels                 | Source |
| --------------- | ---------------------- | ------ |
| `cpu.usage_pct` | `core=all`, `core=N`   | `/proc/stat` |

Busy share of CPU time since the previous sample: `1 − Δidle / Δtotal`. Idle includes `iowait`. `steal` counts as busy; `guest` time is already included in `user` and not double counted. `core=all` is the whole machine; `core=N` is each logical CPU.

## Memory – `mem` collector

| Metric                | Labels | Source |
| --------------------- | ------ | ------ |
| `mem.total_bytes`     | –      | `MemTotal` in `/proc/meminfo` |
| `mem.available_bytes` | –      | `MemAvailable` |
| `mem.used_bytes`      | –      | `MemTotal − MemAvailable` |
| `swap.total_bytes`    | –      | `SwapTotal` |
| `swap.used_bytes`     | –      | `SwapTotal − SwapFree` |

`mem.used_bytes` matches the "used" figure in `free` and most desktop monitors: memory the kernel can't reclaim. Page cache is not counted as used.

## Load – `load` collector

| Metric           | Labels | Source |
| ---------------- | ------ | ------ |
| `load.1m`        | –      | `/proc/loadavg` field 1 |
| `load.5m`        | –      | `/proc/loadavg` field 2 |
| `load.15m`       | –      | `/proc/loadavg` field 3 |
| `uptime.seconds` | –      | `/proc/uptime` field 1 |

## Disk space – `disk` collector

| Metric             | Labels       | Source |
| ------------------ | ------------ | ------ |
| `disk.total_bytes` | `mount=PATH` | `statfs(2)` |
| `disk.used_bytes`  | `mount=PATH` | `(blocks − free blocks) × block size` |
| `disk.free_bytes`  | `mount=PATH` | blocks available to unprivileged users |
| `disk.used_pct`    | `mount=PATH` | `used / (used + free) × 100` |

- Only filesystems backed by a `/dev/...` device are included. `tmpfs`, `overlay`, `proc`, and other virtual filesystems are skipped.
- A device mounted more than once (btrfs subvolumes, bind mounts) is reported once, under its first mount point in `/proc/mounts`.
- `disk.used_pct` matches `df`: blocks reserved for root count as neither used nor free, so `used + free` can be less than `total`.

## Disk IO – `diskio` collector

| Metric                       | Labels          | Source |
| ---------------------------- | --------------- | ------ |
| `diskio.read_bytes_per_sec`  | `device=NAME`   | sectors read in `/proc/diskstats` × 512 |
| `diskio.write_bytes_per_sec` | `device=NAME`   | sectors written × 512 |

- Only whole devices (entries in `/sys/block`) are reported, so partitions never double count.
- Excluded by default: `loop*`, `ram*`, `dm-*`. Configure with [`[diskio] exclude_devices`](configuration.md#diskio).
- `/proc/diskstats` always counts 512-byte sectors, regardless of the drive's physical sector size.

## Network – `net` collector

| Metric                  | Labels         | Source |
| ----------------------- | -------------- | ------ |
| `net.rx_bytes_per_sec`  | `iface=NAME`   | receive bytes in `/proc/net/dev` |
| `net.tx_bytes_per_sec`  | `iface=NAME`   | transmit bytes in `/proc/net/dev` |

Excluded by default: `lo`, `veth*`, `br-*`, `docker*`. Configure with [`[net] exclude_interfaces`](configuration.md#net).

## Temperature – `temp` collector

| Metric         | Labels                              | Source |
| -------------- | ----------------------------------- | ------ |
| `temp.celsius` | `chip=NAME,device=ADDR,sensor=NAME` | `/sys/class/hwmon/hwmon*/temp*_input` |

| Label    | From | Example |
| -------- | ---- | ------- |
| `chip`   | `hwmonN/name` | `coretemp`, `nvme`, `k10temp`, `spd5118` |
| `device` | basename of the `hwmonN/device` symlink | `coretemp.0`, `nvme0`, `6-0051` |
| `sensor` | `tempN_label`, or `tempN` when unlabeled | `Package id 0`, `Composite`, `temp1` |

`device` separates identical chips, such as one `spd5118` sensor per RAM stick, and stays the same across reboots, unlike `hwmonN` numbering. It is omitted for sensors without a device link. Sensors that return an error (for example a suspended GPU) are skipped for that cycle.

Virtual machines and containers usually expose no hwmon sensors; disable the collector there.

## Typical series count

A 12-thread desktop with two mounts, two disks, three network interfaces, and 15 sensors reports about **55 series**:

| Collector | Series |
| --------- | ------ |
| cpu       | cores + 1 (13) |
| mem       | 5 |
| load      | 4 |
| disk      | 4 × mounts (8) |
| diskio    | 2 × devices (4) |
| net       | 2 × interfaces (6) |
| temp      | sensors (15) |

Use `metricsctl list` to see your own counts.
