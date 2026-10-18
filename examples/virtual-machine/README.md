# virtual-machine

For guests: local VMs, VPS instances, and cloud servers (EC2, Hetzner, DigitalOcean, and so on). metricsd detects the VM automatically (`environment = vm`) and reports the VM's own vCPUs and RAM, not the host's.

## Settings

| Key | Value | Why |
| --- | ----- | --- |
| `interval` | `10s` | Half the default storage, still fine-grained enough for incident review. |
| `retention` | `14d` | Two weeks, so "was it slow last Tuesday?" has an answer. |
| `[diskio] exclude_devices` | adds `zram*`, `sr*` | Compressed RAM swap and virtual CD-ROM drives (cloud-init seed images) aren't real disks. |

The `temp`, `fan`, and `battery` collectors stay on. Hypervisors rarely pass sensors through, so they simply report nothing, and they cost nothing. If your hypervisor does expose virtual sensors you don't want, add:

```toml
[collectors]
temp = false
fan  = false
```

## Footprint

About 75 series on a 4-vCPU guest → **~22 MiB/day, ~300 MiB at full retention**.

## Reading CPU in a guest

`cpu.mode_pct{mode=steal}` is time your vCPU was ready to run but the hypervisor ran someone else. `cpu.usage_pct` counts steal as busy. Sustained steal usually means a noisy neighbour or an oversubscribed host.

```bash
metricsctl history cpu.mode_pct -since 6h -step 10m -labels mode=steal
```

## Identity

`metricsctl info` shows `Environment vm` and, when recognizable, the hypervisor or cloud (`kvm`, `vmware`, `hyperv`, `aws`, `gce`, …). The ID is the guest's `/etc/machine-id`. Images cloned without regenerating it share an ID; run `systemd-machine-id-setup` after cloning.

## Running on a headless server

```bash
loginctl enable-linger "$USER"
systemctl --user enable --now metricsd
```

The API is a local Unix socket and isn't reachable from the internet. To ship metrics to a central server, add an [OTLP export](../otlp-http/). To read them by hand, SSH in and run `metricsctl`, or forward the socket:

```bash
ssh -L /tmp/vps-metricsd.sock:/run/user/1000/metricsd/metricsd.sock user@vps
metricsctl -socket /tmp/vps-metricsd.sock now
```

## Variations

- **Tiny VPS (≤ 10 GB disk):** `interval = "30s"`, `retention = "7d"` (~50 MiB total).
- **Disk names:** guests use `vda` (virtio), `xvda` (Xen), `sda`, or `nvme0n1` (EC2 Nitro). All are recorded automatically.
