# virtual-machine

For guests: local VMs, VPS instances, and cloud servers (EC2, Hetzner, DigitalOcean, and so on).

## Settings

| Key | Value | Why |
| --- | ----- | --- |
| `interval` | `10s` | Half the default storage, still fine-grained enough for incident review. |
| `retention` | `14d` | Two weeks, so "was it slow last Tuesday?" has an answer. |
| `[collectors] temp` | `false` | Guests have no hwmon sensors, so the collector would always return nothing. |
| `[diskio] exclude_devices` | adds `zram*`, `sr*` | Compressed RAM swap and virtual CD-ROM drives (cloud-init seed images) aren't real disks. |

## Footprint

About 20 series on a 4-vCPU guest → **~6 MiB/day, ~80 MiB at full retention**.

## Reading CPU in a guest

`cpu.usage_pct` counts **steal** time as busy. Steal is time your vCPU was ready to run but the hypervisor ran someone else. Sustained high CPU while your own processes are idle usually means a noisy neighbour or an oversubscribed host. Compare with `top`, whose `st` column shows steal directly.

## Running on a headless server

```bash
loginctl enable-linger "$USER"
systemctl --user enable --now metricsd
```

The API is a local Unix socket and isn't reachable from the internet, so there's nothing to firewall. To read metrics remotely, SSH in and run `metricsctl`, or forward the socket:

```bash
ssh -L /tmp/vps-metricsd.sock:/run/user/1000/metricsd/metricsd.sock user@vps
metricsctl -socket /tmp/vps-metricsd.sock now
```

## Variations

- **Tiny VPS (≤ 10 GB disk):** `interval = "30s"`, `retention = "7d"` (~10 MiB total).
- **Disk names:** guests use `vda` (virtio), `xvda` (Xen), `sda`, or `nvme0n1` (EC2 Nitro). All are recorded automatically.
