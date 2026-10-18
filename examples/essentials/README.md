# essentials

Records only the four signals that answer "is this machine healthy?": CPU, memory, load, and free disk space.

## Settings

| Collector | State | Notes |
| --------- | ----- | ----- |
| `cpu`     | on  | Overall and per-core usage |
| `mem`     | on  | RAM and swap |
| `load`    | on  | Load averages and uptime |
| `disk`    | on  | Space per mount |
| `diskio`  | off | Throughput per disk |
| `net`     | off | Throughput per interface |
| `temp`    | off | Hardware sensors |

All four `true` lines could be left out, since collectors default to on. They're listed so the file reads as a complete picture.

## Footprint

About 30 series on a 12-thread machine → **~17 MiB/day, ~120 MiB at full retention**. Most of what remains is per-core CPU; on a 4-core machine it drops to about 20 series.

## When to use it

- Small VPS or Raspberry Pi with limited storage.
- You already monitor network or temperature elsewhere.
- You want the simplest possible dashboard source.

## Variations

- **Even smaller:** add `interval = "30s"` (~3 MiB/day).
- **Re-enable one:** flip a single line, e.g. `net = true`; the rest are unaffected.
