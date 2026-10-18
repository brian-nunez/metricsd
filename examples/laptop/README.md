# laptop

Settings for battery-powered machines: fewer wakeups, less disk churn, and no series for hardware that's usually disconnected.

## Settings

| Key | Value | Why |
| --- | ----- | --- |
| `interval` | `30s` | Each sample wakes the CPU briefly and writes to disk. Sampling 6× less often cuts both, with little loss of insight on a personal machine. |
| `retention` | `3d` | Enough to answer "what drained my battery yesterday?" while staying small on limited SSDs. |
| `[net] exclude_interfaces` | adds `en*`, `eth*` | Wired Ethernet is usually unplugged; its all-zero series is noise. Wi-Fi (`wl*`) and VPNs (`tailscale0`, `wg*`) stay. |

All hardware collectors stay on; on a laptop they are the most useful signals:

| Metric | Labels | Meaning |
| ------ | ------ | ------- |
| `battery.charge_pct` | `battery=BAT0` | Charge level |
| `battery.charging` | `battery=BAT0` | `1` while charging, else `0` |
| `battery.power_watts` | `battery=BAT0` | Current draw (or charge rate) |
| `battery.health_pct` | `battery=BAT0` | Full capacity vs. design capacity |
| `battery.cycles` | `battery=BAT0` | Charge cycles, when the firmware reports it |
| `power.ac_online` | `supply=AC` | `1` when plugged in |
| `fan.rpm` | `chip=...,sensor=...` | Fan speed, when the firmware exposes it (ThinkPads, many gaming laptops) |
| `temp.celsius` | `chip=...,sensor=...` | CPU package, cores, NVMe, battery |

Peripheral batteries (wireless mice, keyboards) are ignored. A battery swapped or docked in later is picked up on the next cycle.

## Footprint

About 150 series → **~15 MiB/day, ~45 MiB at full retention**.

## Useful queries

```bash
# Battery drain over the day, 30-minute averages
metricsctl history battery.power_watts -since 12h -step 30m

# How fast did the charge drop?
metricsctl history battery.charge_pct -since 8h -step 15m

# Did the CPU run hot (and the fan spin up) during that video call?
metricsctl history temp.celsius -since 3h -step 5m
metricsctl history fan.rpm -since 3h -step 5m

# Wi-Fi throughput (interface name from `ip -br link`)
metricsctl history net.rx_bytes_per_sec -since 1h -labels iface=wlp4s0
```

## Variations

- **Docking station user:** remove `en*` from the list so the dock's Ethernet is recorded.
- **Even lighter:** `interval = "60s"` and `[collectors] diskio = false`.
- **Docker on the laptop:** the default Docker patterns are already included.
