# laptop

Settings for battery-powered machines: fewer wakeups, less disk churn, and no series for hardware that's usually disconnected.

## Settings

| Key | Value | Why |
| --- | ----- | --- |
| `interval` | `30s` | Each sample wakes the CPU briefly and writes to disk. Sampling 6× less often cuts both, with little loss of insight on a personal machine. |
| `retention` | `3d` | Enough to answer "what drained my battery yesterday?" while staying small on limited SSDs. |
| `[net] exclude_interfaces` | adds `en*`, `eth*` | Wired Ethernet is usually unplugged; its all-zero series is noise. Wi-Fi (`wl*`) and VPNs (`tailscale0`, `wg*`) stay. |

The `temp` collector stays on — CPU and NVMe temperatures are the most useful laptop signals.

## Footprint

About 50 series → **~4.5 MiB/day, ~14 MiB at full retention**.

## Useful queries

```bash
# Did the CPU run hot during that video call?
metricsctl history temp.celsius -since 3h -step 5m

# What's been eating memory today?
metricsctl history mem.used_bytes -since 12h -step 30m

# Wi-Fi throughput (interface name from `ip -br link`)
metricsctl history net.rx_bytes_per_sec -since 1h -labels iface=wlp4s0
```

## Variations

- **Docking station user:** remove `en*` from the list so the dock's Ethernet is recorded.
- **Even lighter:** `interval = "60s"` and `[collectors] diskio = false`.
- **Docker on the laptop:** the default Docker patterns are already included.
