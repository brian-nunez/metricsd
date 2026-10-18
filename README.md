# metricsd

A lightweight Linux metrics agent. It gathers metrics from **the machine or container it runs in**, keeps a rolling history in SQLite, and exports to a central server over **OTLP** (HTTP or gRPC). `metricsctl` is the local command-line client.

- **Measures its own environment** – physical machine, VM, Docker, Podman, or Kubernetes, detected automatically. A container on a VM on a server is three independent sources.
- **Rootless-ready** – works under rootless Docker and Podman; missing cgroup controllers mean missing metrics, never fake zeros
- **~90 metrics** – CPU, memory, swap, disk space and IO, network, TCP/UDP, pressure stall, processes, temperatures, fans, battery
- **OTLP export** – HTTP or gRPC, TLS and mTLS, header auth; identity sent as standard resource attributes
- **Buffered delivery** – the local database is the export buffer; outages are backfilled with original timestamps
- **Local and private API** – JSON over a `0600` Unix socket, never a network port
- **Static binaries** – no CGO; a 38 MB `scratch` image to copy into any container
- **Hardened systemd user unit** – read-only filesystem, restricted sockets, syscall filtering

## Quick start

### On a machine (PC, server, VM)

Requires Linux, Go 1.27+, `make`, and a systemd user session.

```bash
git clone git@github.com:brian-nunez/metricsd.git
cd metricsd
make install
systemctl --user enable --now metricsd
metricsctl info
metricsctl now
```

Make sure `~/.local/bin` is on your `PATH`. See [docs/installation.md](docs/installation.md).

### In a container

```bash
docker build -t metricsd .
```

```dockerfile
# your application's Dockerfile
COPY --from=metricsd:latest /usr/local/bin/ /usr/local/bin/
CMD ["sh", "-c", "metricsd & exec myapp"]
```

```bash
docker exec myapp metricsctl now
```

See [docs/containers.md](docs/containers.md).

### Send to a central server

```toml
# ~/.config/metricsd/config.toml
[tags]
env = "prod"

[export.otlp]
enabled  = true
protocol = "grpc"
endpoint = "https://otel.example.com:4317"
```

See [docs/export.md](docs/export.md).

## What it looks like

```console
$ metricsctl info
Environment    docker
ID             b222ab9813a46ecdd4dbfbfa62d4e217a35058a277ef963b671e1431ca76b9f6
OS             alpine 3.24.2 (linux/amd64)
...

$ metricsctl now
METRIC                      LABELS          VALUE
cpu.cores                                   1
cpu.usage_pct               core=all        99.9%
cpu.throttled_periods_pct                   6.0%
mem.total_bytes                             1.0 GiB
mem.used_bytes                              11.3 MiB
net.rx_bytes_per_sec        iface=eth0      680 B/s
pressure.some_pct           resource=cpu    12.7%
...
```

The same data over the local API:

```bash
curl -s --unix-socket "$XDG_RUNTIME_DIR/metricsd/metricsd.sock" http://x/v1/metrics | jq
```

## Metrics

| Group | Examples | Machines | Containers |
| ----- | -------- | :------: | :--------: |
| CPU | usage per core and by mode, cores, frequency, context switches, throttling | ✓ | ✓ (vs. its CPU limit) |
| Memory | used, available, cached, slab, dirty, page faults, OOM kills | ✓ | ✓ (vs. its memory limit) |
| Swap | used, free, in/out rates | ✓ | ✓ |
| Load | 1/5/15 minute averages | ✓ | – |
| Disk | space and inodes per mount | ✓ | ✓ (root and volumes) |
| Disk IO | throughput, IOPS, latency, busy % | ✓ | ✓ (its own IO) |
| Network | bytes, packets, errors, drops per interface | ✓ | ✓ |
| TCP/UDP | connections, retransmits, resets, datagrams, sockets | ✓ | ✓ |
| Pressure | CPU, memory, IO stall % | ✓ | ✓ |
| System | uptime, processes, threads, open files, users | ✓ | ✓ (uptime, processes, task limit) |
| Sensors | temperatures, fan RPM, battery, AC power | ✓ | – |

Full reference: [docs/metrics.md](docs/metrics.md).

## Documentation

| Doc | Covers |
| --- | ------ |
| [Installation](docs/installation.md) | Requirements, `make install`, Docker image, manual install |
| [Containers](docs/containers.md) | Docker, Podman, Kubernetes, rootless, adding to images |
| [Export](docs/export.md) | OTLP HTTP/gRPC, TLS, auth, resource attributes, buffering |
| [Usage](docs/usage.md) | Every `metricsctl` command and flag |
| [Configuration](docs/configuration.md) | Every config key, defaults, validation |
| [Metrics](docs/metrics.md) | Metric names, labels, units, sources |
| [API](docs/api.md) | Endpoints, parameters, JSON schemas, errors |
| [systemd](docs/systemd.md) | Running as a service, sandboxing, drop-ins, logs |
| [Architecture](docs/architecture.md) | How it works, storage schema, disk usage sizing |
| [Troubleshooting](docs/troubleshooting.md) | Common errors and fixes |
| [Uninstall](docs/uninstall.md) | Removing binaries, service, and data |
| [Examples](examples/README.md) | Annotated example configs |

## Project layout

```
cmd/metricsd/          daemon entry point and collector registry
cmd/metricsctl/        CLI client
internal/collector/    collectors: host (gopsutil, /proc, /sys) and container (cgroup v2)
internal/environment/  physical / VM / Docker / Podman / Kubernetes detection
internal/identity/     who is being measured: IDs, OS, container and pod details
internal/export/       exporter interface and buffered delivery loop
internal/export/otlp/  OTLP HTTP and gRPC exporter
internal/store/        SQLite storage, retention, export cursors
internal/api/          HTTP handlers and Unix socket listener
internal/config/       TOML config, defaults, validation
internal/wire/         JSON types and units shared by daemon and client
internal/version/      build version
deploy/                systemd unit and reference config
examples/              example configs with explanations
docs/                  user documentation
Dockerfile             minimal image for copying into application images
```

## License

[MIT](LICENSE) © 2026 Brian Nunez
