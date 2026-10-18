# essentials

Records only the signals that answer "is this machine healthy?": CPU, memory, load, free disk space, and uptime.

## Settings

| Collector  | State | Notes |
| ---------- | ----- | ----- |
| `cpu`      | on  | Usage overall and per core, mode breakdown, frequency |
| `mem`      | on  | RAM, swap, paging |
| `load`     | on  | Load averages (machines only) |
| `disk`     | on  | Space and inodes per mount |
| `system`   | on  | Uptime, process and thread counts, open files |
| `diskio`   | off | Throughput, IOPS, latency per disk |
| `net`      | off | Throughput per interface |
| `netstat`  | off | TCP/UDP counters and socket usage |
| `pressure` | off | CPU/memory/IO stall percentages |
| `temp`     | off | Temperature sensors |
| `fan`      | off | Fan speeds |
| `battery`  | off | Battery charge and AC power |

`system` stays on because uptime is the cheapest way to spot unexpected reboots or container restarts, and it adds only about ten series. Turn it off too for the absolute minimum.

The `true` lines could be left out, since collectors default to on. They're listed so the file reads as a complete picture; every collector must be listed as `false` to be turned off.

## Footprint

About 75 series on a 12-thread machine → **~43 MiB at full retention** (1 day). Most of what remains is per-core CPU usage and frequency; on a 4-core machine it drops to about 50 series. Inside a container it is about 30 series.

## When to use it

- Small VPS or Raspberry Pi with limited storage.
- You already monitor network, disks, or hardware elsewhere.
- You want the simplest possible dashboard source.

## Variations

- **Even smaller:** add `interval = "30s"` (~7 MiB/day).
- **Re-enable one:** flip a single line, e.g. `net = true`; the rest are unaffected.
