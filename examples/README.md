# Example configurations

Each directory holds a `config.toml` and a README explaining what it changes and why. Every example is validated against the real config loader.

| Example | For | Key settings |
| ------- | --- | ------------ |
| [minimal](minimal/) | Getting started | Only the keys people most often change |
| [laptop](laptop/) | Battery-powered machines | 30s interval, 3 days, Wi-Fi only |
| [high-resolution](high-resolution/) | Benchmarking, debugging spikes | 1s interval, 1 day |
| [long-term](long-term/) | Capacity planning, trends | 60s interval, 90 days |
| [essentials](essentials/) | Smallest footprint | CPU, memory, load, disk space only |
| [docker-host](docker-host/) | Machines running Docker | Keeps `docker0`, drops per-container interfaces |
| [kubernetes-node](kubernetes-node/) | k3s / k8s nodes | Drops CNI interfaces (Calico, Flannel, Cilium) |
| [virtualization-host](virtualization-host/) | libvirt / KVM / Proxmox hosts | Drops VM tap interfaces |
| [virtual-machine](virtual-machine/) | Guests, VPS, cloud instances | No temperature sensors |
| [encrypted-storage](encrypted-storage/) | LUKS / LVM setups | Measures `dm-*` devices instead of raw disks |
| [custom-paths](custom-paths/) | Database on another disk | Custom `db_path` plus systemd drop-in |

## Using an example

```bash
mkdir -p ~/.config/metricsd
cp examples/laptop/config.toml ~/.config/metricsd/config.toml
systemctl --user restart metricsd
journalctl --user -u metricsd -n 5    # confirm "metricsd started"
```

Or keep several configs and point the daemon at one with a [drop-in](../docs/systemd.md#use-a-different-config-file).

## Combining examples

Every key is optional, so examples combine by merging their keys. When two examples set the same list (such as `exclude_interfaces`), merge the lists by hand, because a list in the file replaces the default list entirely.

```toml
# laptop + docker-host
interval  = "30s"
retention = "3d"

[net]
exclude_interfaces = ["lo", "veth*", "br-*", "en*", "eth*"]
```

## Trying a config without touching your setup

Run a second copy in the foreground with throwaway paths:

```bash
cat > /tmp/try.toml <<'EOF'
db_path     = "/tmp/metricsd-try/metrics.db"
socket_path = "/tmp/metricsd-try/metricsd.sock"
EOF
cat examples/laptop/config.toml >> /tmp/try.toml   # paths stay first; example keys follow
./bin/metricsd -config /tmp/try.toml &
./bin/metricsctl -socket /tmp/metricsd-try/metricsd.sock now
kill %1
```

This works for examples that don't set `db_path` or `socket_path` themselves. A duplicate key is an error, so remove any repeats before running.

Full key reference: [docs/configuration.md](../docs/configuration.md).
