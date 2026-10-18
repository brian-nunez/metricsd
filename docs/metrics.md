# Metrics reference

Each value metricsd stores is a **sample**: a metric name, a label string, and a number. A metric plus one specific label string is a **series** (for example `cpu.usage_pct` with `core=3`).

metricsd measures **the environment it runs in**. On a physical machine or VM it reads the machine-wide kernel interfaces (`/proc`, `/sys`). Inside a container it reads the container's own cgroup, so a container limited to 1 GiB and 1 CPU reports 1 GiB and 1 CPU, not the host's capacity. See [containers.md](containers.md) for how the environment is detected.

## Naming

- Names are `group.name_unit`, for example `mem.used_bytes`. Names are identical in every environment.
- Labels are comma-separated `key=value` pairs, for example `chip=nvme,device=nvme0,sensor=Composite`.
- Commas and `=` inside label **values** are replaced with `_`, so exporters can split labels back into pairs unambiguously.
- Metrics with a single series have an empty label string.

## Units

The unit comes from the name's suffix (`wire.Unit` in the code). `metricsctl` formats values from it, and OTLP export sends it as the metric's UCUM unit. The API always returns raw numbers.

| Suffix            | UCUM unit | Raw value       | `metricsctl` shows |
| ----------------- | --------- | --------------- | ------------------ |
| `_bytes_per_sec`  | `By/s`    | bytes/second    | `17.1 MiB/s` |
| `_bytes`          | `By`      | bytes           | `7.9 GiB` (binary prefixes) |
| `_pct`            | `%`       | percent 0–100   | `27.9%` |
| `_per_sec`        | `1/s`     | events/second   | `6758.5/s` |
| `_mhz`            | `MHz`     | megahertz       | `1418 MHz` |
| `_ms`             | `ms`      | milliseconds    | `0.02 ms` |
| `_watts`          | `W`       | watts           | `11.2 W` |
| `.celsius`        | `Cel`     | degrees C       | `38.9°C` |
| `.seconds`        | `s`       | seconds         | `6d 16h 8m` |
| `.rpm`            | `1/min`   | revolutions/min | `2650 RPM` |
| anything else     | `1`       | count or ratio  | `433`, `0.62` |

## Gauges and rates

- **Gauges** (memory, disk space, load, temperature, counts) are read as-is each cycle.
- **Rates** (`*_per_sec`, CPU %, latency, busy %, pressure %) come from counters that only increase. metricsd keeps the previous reading, takes the difference, and divides by elapsed time.
- Rates therefore appear from the **second** cycle on.
- A counter that goes backwards (device removed and re-added, counter reset, `iowait` ticking back on some kernels) is skipped for one cycle rather than reported as a bogus value.

## Availability by environment

| Collector  | Physical / VM | Container |
| ---------- | ------------- | --------- |
| `cpu`      | `/proc/stat`, cpufreq | cgroup `cpu.stat`, `cpu.max` |
| `mem`      | `/proc/meminfo`, `/proc/vmstat` | cgroup `memory.*` |
| `load`     | `/proc/loadavg` | not collected (machine-wide only) |
| `disk`     | block-device filesystems | root overlay plus mounted volumes |
| `diskio`   | `/proc/diskstats` | cgroup `io.stat` |
| `net`      | `/proc/net/dev` | same file, scoped to the container's network namespace |
| `netstat`  | `/proc/net/snmp`, `/proc/net/sockstat` | same, minus machine-wide socket figures |
| `system`   | uptime, processes, threads, file handles, users | container uptime, processes, task limit |
| `pressure` | `/proc/pressure/*` | cgroup `*.pressure` |
| `temp`     | hwmon | not collected (host hardware) |
| `fan`      | hwmon | not collected (host hardware) |
| `battery`  | `/sys/class/power_supply` | not collected (host hardware) |

### Containers and rootless runtimes

Every cgroup file is optional. Rootless Docker and Podman often get only the `memory` and `pids` controllers delegated (`cpu` and `io` need extra systemd configuration). When a controller's files are missing:

- the collector that needs them is **skipped** at startup with a log line such as `collector skipped collector=diskio reason="io controller not delegated to /sys/fs/cgroup"`;
- individual metrics that need a missing file are omitted;
- nothing is ever reported as zero just because it couldn't be read.

`cpu.stat` exists in every cgroup v2, so CPU usage is always available. cgroup v1 hosts are not supported for container metrics; see [containers.md](containers.md).

### Hardware collectors

On physical machines and VMs, `temp`, `fan`, and `battery` always run. They rediscover hardware every cycle, so a battery that is hot-swapped, a UPS plugged in later, or a sensor driver that loads late is picked up within one interval. Machines without the hardware (desktops without batteries, VMs without sensors) simply report nothing from them.

## CPU – `cpu` collector

### Physical / VM

| Metric | Labels | Source |
| ------ | ------ | ------ |
| `cpu.cores` | – | logical CPUs |
| `cpu.physical_cores` | – | physical cores (omitted when unknown) |
| `cpu.usage_pct` | `core=all`, `core=N` | `/proc/stat` |
| `cpu.mode_pct` | `mode=user\|nice\|system\|idle\|iowait\|irq\|softirq\|steal` | `/proc/stat`, whole machine |
| `cpu.frequency_mhz` | `core=N` | `/sys/devices/system/cpu/cpuN/cpufreq/scaling_cur_freq` |
| `cpu.context_switches_per_sec` | – | `ctxt` in `/proc/stat` |
| `cpu.interrupts_per_sec` | – | `intr` in `/proc/stat` |
| `cpu.softirqs_per_sec` | – | `softirq` in `/proc/stat` |

- `cpu.usage_pct` is `1 − Δidle / Δtotal`. Idle includes `iowait`; `steal` counts as busy (in a VM, sustained steal means the hypervisor is oversubscribed). `guest` time is already inside `user` and is not double counted.
- `cpu.mode_pct` values for one cycle add up to 100%.
- `cpu.frequency_mhz` is missing on VMs and boards without cpufreq.

### Container

| Metric | Labels | Source |
| ------ | ------ | ------ |
| `cpu.cores` | – | quota from `cpu.max`, capped at the CPUs in the container's cpuset |
| `cpu.usage_pct` | `core=all` | `usage_usec` in `cpu.stat` |
| `cpu.mode_pct` | `mode=user\|system` | `user_usec`, `system_usec` |
| `cpu.throttled_periods_pct` | – | `nr_throttled / nr_periods` |
| `cpu.throttled_time_pct` | – | `throttled_usec` as a share of wall time |

- Usage is a percentage of the container's own allowance: `--cpus=1` on a 64-core host shows `cpu.cores 1` and `100%` when the container uses one full CPU. Without a quota, the allowance is the CPUs the container can run on.
- Throttling metrics need the `cpu` controller (and a quota for the periods metric). They are omitted in rootless containers without `cpu` delegation.
- `cpu.cores` can be fractional (`--cpus=0.5` → `0.50`).

## Memory – `mem` collector

### Physical / VM

| Metric | Source (`/proc/meminfo` unless noted) |
| ------ | ------ |
| `mem.total_bytes` | `MemTotal` |
| `mem.available_bytes` | `MemAvailable` |
| `mem.used_bytes` | `MemTotal − MemAvailable` |
| `mem.used_pct` | used / total |
| `mem.free_bytes` | `MemFree` |
| `mem.buffers_bytes` | `Buffers` |
| `mem.cached_bytes` | `Cached` |
| `mem.shared_bytes` | `Shmem` |
| `mem.slab_bytes` | `Slab` |
| `mem.active_bytes` | `Active` |
| `mem.inactive_bytes` | `Inactive` |
| `mem.dirty_bytes` | `Dirty` |
| `mem.writeback_bytes` | `Writeback` |
| `mem.mapped_bytes` | `Mapped` |
| `mem.page_tables_bytes` | `PageTables` |
| `mem.committed_bytes` | `Committed_AS` |
| `mem.commit_limit_bytes` | `CommitLimit` |
| `mem.hugepages_total`, `mem.hugepages_free` | `HugePages_*` (only when huge pages are configured) |
| `swap.total_bytes`, `swap.used_bytes`, `swap.free_bytes`, `swap.used_pct` | `SwapTotal`, `SwapFree` |
| `swap.cached_bytes` | `SwapCached` |
| `swap.in_bytes_per_sec`, `swap.out_bytes_per_sec` | `pswpin`, `pswpout` in `/proc/vmstat` |
| `mem.page_faults_per_sec`, `mem.major_page_faults_per_sec` | `pgfault`, `pgmajfault` in `/proc/vmstat` |

`mem.used_bytes` matches the "used" figure in `free`: memory the kernel can't reclaim. Page cache is available, not used.

### Container

| Metric | Source (cgroup) |
| ------ | ------ |
| `mem.total_bytes` | `memory.max`; the host's total when unlimited or larger than the host |
| `mem.used_bytes` | `memory.current − inactive_file` (same as `docker stats`) |
| `mem.available_bytes`, `mem.used_pct` | from total and used |
| `mem.cached_bytes` | `file` in `memory.stat` |
| `mem.anon_bytes` | `anon` |
| `mem.kernel_bytes` | `kernel` (or `kernel_stack + slab` on older kernels) |
| `mem.slab_bytes`, `mem.shared_bytes`, `mem.mapped_bytes` | `slab`, `shmem`, `file_mapped` |
| `mem.dirty_bytes`, `mem.writeback_bytes` | `file_dirty`, `file_writeback` |
| `mem.socket_bytes` | `sock` |
| `mem.peak_bytes` | `memory.peak` (kernel 5.19+) |
| `mem.oom_kills` | `oom_kill` in `memory.events` (cumulative count) |
| `swap.total_bytes`, `swap.used_bytes`, `swap.free_bytes`, `swap.used_pct` | `memory.swap.current`, `memory.swap.max` (capped at host swap) |
| `mem.page_faults_per_sec`, `mem.major_page_faults_per_sec` | `pgfault`, `pgmajfault` |

- The collector needs the `memory` controller (`memory.current`); without it the collector is skipped.
- Swap metrics appear only when the host has swap accounting enabled.

## Load – `load` collector

Physical / VM only. Load average is a machine-wide kernel figure with no per-container equivalent; use `pressure.*` inside containers.

| Metric     | Source |
| ---------- | ------ |
| `load.1m`  | `/proc/loadavg` field 1 |
| `load.5m`  | `/proc/loadavg` field 2 |
| `load.15m` | `/proc/loadavg` field 3 |

## Disk space – `disk` collector

| Metric | Labels | Source |
| ------ | ------ | ------ |
| `disk.total_bytes` | `mount=PATH` | `statfs(2)` |
| `disk.used_bytes` | `mount=PATH` | `(blocks − free blocks) × block size` |
| `disk.free_bytes` | `mount=PATH` | blocks available to unprivileged users |
| `disk.used_pct` | `mount=PATH` | `used / (used + free) × 100`, matching `df` |
| `disk.inodes_total`, `disk.inodes_used`, `disk.inodes_free`, `disk.inodes_used_pct` | `mount=PATH` | inode counts (omitted on filesystems that report none, such as btrfs) |

- **Physical / VM:** only filesystems backed by a `/dev/...` device. `tmpfs`, `overlay`, `proc`, and network filesystems are skipped. A device mounted several times (btrfs subvolumes, bind mounts) is reported once, under its first mount point.
- **Container:** the container's root filesystem (`overlay`, or `fuse-overlayfs` when rootless) is reported as `mount=/`, plus any mounted volumes. Single files bind-mounted from the host (`/etc/hosts`, `/etc/hostname`, `/etc/resolv.conf`) are skipped because they describe the host's disk. The root overlay's capacity is the capacity of the storage backing it on the host.

## Disk IO – `diskio` collector

### Physical / VM

| Metric | Labels | Source |
| ------ | ------ | ------ |
| `diskio.read_bytes_per_sec`, `diskio.write_bytes_per_sec` | `device=NAME` | `/proc/diskstats` sectors × 512 |
| `diskio.reads_per_sec`, `diskio.writes_per_sec` | `device=NAME` | completed requests (IOPS) |
| `diskio.read_latency_ms`, `diskio.write_latency_ms` | `device=NAME` | average time per completed request (like iostat `r_await`/`w_await`); `0` when idle |
| `diskio.busy_pct` | `device=NAME` | time the device had IO in flight (iostat `%util`) |
| `diskio.in_progress` | `device=NAME` | requests in flight right now |

### Container

| Metric | Labels | Source |
| ------ | ------ | ------ |
| `diskio.read_bytes_per_sec`, `diskio.write_bytes_per_sec` | `device=NAME` | `rbytes`, `wbytes` in `io.stat` |
| `diskio.reads_per_sec`, `diskio.writes_per_sec` | `device=NAME` | `rios`, `wios` |

- Only this container's IO is counted. Devices are named from `/sys/dev/block/MAJ:MIN` (falling back to `MAJ:MIN` when `/sys` isn't visible).
- Needs the `io` controller; skipped in rootless containers without `io` delegation.
- In both environments only whole devices are reported (partitions never double count), and `[diskio] exclude_devices` applies (default `loop*`, `ram*`, `dm-*`). See [configuration.md](configuration.md#diskio).

## Network – `net` collector

| Metric | Labels | Source |
| ------ | ------ | ------ |
| `net.rx_bytes_per_sec`, `net.tx_bytes_per_sec` | `iface=NAME` | `/proc/net/dev` |
| `net.rx_packets_per_sec`, `net.tx_packets_per_sec` | `iface=NAME` | packets |
| `net.rx_errors_per_sec`, `net.tx_errors_per_sec` | `iface=NAME` | errors |
| `net.rx_drops_per_sec`, `net.tx_drops_per_sec` | `iface=NAME` | dropped packets |

`/proc/net/dev` is scoped to the network namespace, so a container sees its own `eth0`. Excluded by default: `lo`, `veth*`, `br-*`, `docker*`. Configure with [`[net] exclude_interfaces`](configuration.md#net).

## Protocols and sockets – `netstat` collector

From `/proc/net/snmp` and `/proc/net/sockstat`, scoped to the network namespace.

| Metric | Source |
| ------ | ------ |
| `tcp.established` | `CurrEstab` (gauge) |
| `tcp.active_opens_per_sec` | `ActiveOpens` (outgoing connections) |
| `tcp.passive_opens_per_sec` | `PassiveOpens` (accepted connections) |
| `tcp.failed_connections_per_sec` | `AttemptFails` |
| `tcp.resets_per_sec` | `EstabResets` |
| `tcp.resets_sent_per_sec` | `OutRsts` |
| `tcp.segments_in_per_sec`, `tcp.segments_out_per_sec` | `InSegs`, `OutSegs` |
| `tcp.retransmits_per_sec` | `RetransSegs` |
| `tcp.errors_in_per_sec` | `InErrs` |
| `udp.datagrams_in_per_sec`, `udp.datagrams_out_per_sec` | `InDatagrams`, `OutDatagrams` |
| `udp.errors_in_per_sec` | `InErrors` |
| `udp.no_port_per_sec` | `NoPorts` |
| `udp.receive_buffer_errors_per_sec`, `udp.send_buffer_errors_per_sec` | `RcvbufErrors`, `SndbufErrors` |
| `sockets.used` | `sockets: used` |
| `tcp.in_use`, `tcp.time_wait`, `udp.in_use` | `TCP: inuse`, `TCP: tw`, `UDP: inuse` |
| `tcp.orphans`, `tcp.allocated` | `TCP: orphan`, `TCP: alloc` (physical / VM only) |
| `tcp.memory_bytes`, `udp.memory_bytes` | `mem` pages × page size (physical / VM only) |

Orphans, allocations, and socket buffer memory are tracked machine-wide by the kernel, so containers omit them.

## System – `system` collector

### Physical / VM

| Metric | Source |
| ------ | ------ |
| `uptime.seconds` | `/proc/uptime` |
| `procs.count` | processes in `/proc` |
| `procs.running`, `procs.blocked` | `procs_running`, `procs_blocked` in `/proc/stat` |
| `procs.threads` | total threads, field 4 of `/proc/loadavg` |
| `procs.created_per_sec` | `processes` in `/proc/stat` (forks) |
| `fs.open_files` | allocated file handles, `/proc/sys/fs/file-nr` |
| `fs.open_files_max` | file handle limit (omitted when the kernel reports no limit) |
| `users.logged_in` | sessions in utmp (omitted on systems without utmp) |

### Container

| Metric | Source |
| ------ | ------ |
| `uptime.seconds` | age of the container's PID 1 |
| `procs.count` | processes in the container's PID namespace |
| `procs.threads` | `pids.current` (every task in the cgroup) |
| `procs.limit` | `pids.max` (omitted when unlimited) |

## Pressure – `pressure` collector

| Metric | Labels | Source |
| ------ | ------ | ------ |
| `pressure.some_pct` | `resource=cpu\|memory\|io` | share of the interval in which at least one task was stalled |
| `pressure.full_pct` | `resource=cpu\|memory\|io` | share of the interval in which all non-idle tasks were stalled |

- Physical / VM read `/proc/pressure/*`; containers read their cgroup's `cpu.pressure`, `memory.pressure`, `io.pressure`.
- Values come from the cumulative `total=` stall time, so they cover exactly the last interval rather than the kernel's fixed 10s/60s/300s averages.
- At machine level, `pressure.full_pct{resource=cpu}` is always `0` on current kernels.
- Skipped when the kernel has no PSI (`CONFIG_PSI` off or booted with `psi=0`) or the cgroup has no pressure files.

## Temperature – `temp` collector

Physical / VM only.

| Metric | Labels | Source |
| ------ | ------ | ------ |
| `temp.celsius` | `chip=NAME,device=ADDR,sensor=NAME` | `/sys/class/hwmon/hwmon*/temp*_input` |

| Label    | From | Example |
| -------- | ---- | ------- |
| `chip`   | `hwmonN/name` | `coretemp`, `nvme`, `k10temp`, `spd5118`, `thinkpad` |
| `device` | basename of the `hwmonN/device` symlink | `coretemp.0`, `nvme0`, `6-0051` |
| `sensor` | `tempN_label`, or `tempN` when unlabeled | `Package id 0`, `Composite`, `temp1` |

`device` separates identical chips, such as one `spd5118` per RAM stick, and stays the same across reboots, unlike `hwmonN` numbering. It is omitted for sensors without a device link. Sensors that return an error (for example a suspended GPU) are skipped for that cycle.

## Fans – `fan` collector

Physical / VM only.

| Metric | Labels | Source |
| ------ | ------ | ------ |
| `fan.rpm` | `chip=NAME,device=ADDR,sensor=NAME` | `/sys/class/hwmon/hwmon*/fan*_input` |

Labels follow the temperature rules (`sensor` is `fanN_label`, e.g. `CPU Fan`, or `fanN`). Many laptops expose no fan sensors; desktop boards usually need their Super I/O driver (`nct6775`, `it87`) loaded.

## Battery and power – `battery` collector

Physical / VM only.

| Metric | Labels | Source (`/sys/class/power_supply/NAME/`) |
| ------ | ------ | ------ |
| `battery.charge_pct` | `battery=NAME` | `capacity` |
| `battery.charging` | `battery=NAME` | `1` when `status` is `Charging`, else `0` |
| `battery.power_watts` | `battery=NAME` | `power_now`, or `current_now × voltage_now` |
| `battery.health_pct` | `battery=NAME` | `energy_full / energy_full_design` (or `charge_*`) |
| `battery.cycles` | `battery=NAME` | `cycle_count` (omitted when the driver reports 0) |
| `power.ac_online` | `supply=NAME` | `online` of each mains adapter |

Only system batteries are reported. Peripheral batteries (wireless mice, keyboards, headsets) have `scope=Device` and are skipped.

## Typical series count

Measured on this project's test machines:

| Environment | Metrics | Series |
| ----------- | ------- | ------ |
| Physical desktop (12 threads, 2 mounts, 2 disks, 3 interfaces, 15 sensors, no battery or fans) | 93 | 169 |
| Docker container (1 CPU, 1 GiB) | 68 | 73 |

Per-core metrics (`cpu.usage_pct`, `cpu.frequency_mhz`) dominate on machines with many cores. Use `metricsctl list` to see your own counts, and [architecture.md](architecture.md#disk-usage) for storage sizing.
