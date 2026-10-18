# high-resolution

Samples every second — the fastest metricsd allows — to catch short CPU, IO, and network bursts that a 5s average would flatten.

## Settings

| Key | Value | Why |
| --- | ----- | --- |
| `interval` | `1s` | The minimum. A 2-second CPU spike shows up as a spike instead of a slight bump. |
| `retention` | `1d` | 1s data is 5× larger than the default. A day covers a benchmark session or "what happened at 3 a.m.?". |

## Footprint

About 55 series → **~160 MiB/day**, which is also the steady-state size with 1-day retention.

CPU cost stays small (a few file reads and one SQLite transaction per second), but this is the setting to avoid on battery.

## When to use it

- Benchmarking or load-testing a service on this machine.
- Hunting intermittent stalls, IO storms, or thermal throttling.
- Short investigations — switch back to the default afterwards.

## Useful queries

```bash
# Every second of the last 5 minutes, overall CPU
metricsctl history cpu.usage_pct -since 5m -step 1s -labels core=all

# Per-core view to spot a single pegged thread
metricsctl history cpu.usage_pct -since 2m -step 1s

# Disk write bursts during a build (device name from `lsblk -d`)
metricsctl history diskio.write_bytes_per_sec -since 10m -step 1s -labels device=nvme0n1
```

`-step` must keep `since / step` at or below 10,000 buckets, so 1-second resolution works for ranges up to about 2h 45m. For longer ranges, use a larger step; full-resolution data stays stored either way.

## Variations

- **Longer investigation:** `retention = "3d"` (~480 MiB).
- **Focus on one subsystem:** turn off unrelated collectors under `[collectors]` to shrink storage proportionally.
