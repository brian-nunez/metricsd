# metricsd

A lightweight Linux system metrics daemon. It samples CPU, memory, disk, network, and temperature data straight from `/proc` and `/sys`, keeps a rolling history in SQLite, and serves it over a local JSON API. `metricsctl` is the command-line client.

- **Zero agents, zero exporters** – reads kernel interfaces directly; no root required
- **History built in** – pure-Go SQLite (no CGO), configurable retention, automatic pruning
- **Local and private** – API listens on a `0600` Unix socket, never a network port
- **Single static binaries** – `metricsd` (~11 MB) and `metricsctl` (~7 MB)
- **Hardened systemd user unit** – read-only filesystem, Unix sockets only, syscall filtering

## Quick start

Requires Linux, Go 1.27+, `make`, and a systemd user session.

```bash
git clone git@github.com:brian-nunez/metricsd.git
cd metricsd
make install
systemctl --user enable --now metricsd
metricsctl now
```

Make sure `~/.local/bin` is on your `PATH`. See [docs/installation.md](docs/installation.md) for details and alternatives.

## What it looks like

```console
$ metricsctl now
Collected 2026-10-17 19:03:19

METRIC                      LABELS                          VALUE
cpu.usage_pct               core=0                          1.0%
cpu.usage_pct               core=all                        1.0%
disk.used_pct               mount=/                         27.9%
mem.used_bytes                                              7.9 GiB
net.rx_bytes_per_sec        iface=enp3s0                    616 B/s
temp.celsius                chip=coretemp,device=coretemp.0,sensor=Package id 0   32.0°C
...

$ metricsctl history cpu.usage_pct --since 6h --step 30m --labels core=all
cpu.usage_pct{core=all}
    13:00:00  4.2%
    13:30:00  3.8%
    ...
```

The same data over the API:

```bash
curl -s --unix-socket "$XDG_RUNTIME_DIR/metricsd/metricsd.sock" http://x/v1/metrics | jq
```

## Metrics

| Group   | Metrics                                                          |
| ------- | ---------------------------------------------------------------- |
| CPU     | usage % per core and overall                                     |
| Memory  | total, used, available; swap total and used                      |
| Load    | 1, 5, 15 minute load averages; uptime                            |
| Disk    | total, used, free, used % per mounted block device               |
| Disk IO | read and write bytes/sec per physical device                     |
| Network | receive and transmit bytes/sec per interface                     |
| Temp    | every hwmon sensor (CPU cores, NVMe, DIMMs, chipset, ...) in °C  |

Full reference: [docs/metrics.md](docs/metrics.md).

## Configuration

Works with no config file. To customize, copy [`deploy/config.example.toml`](deploy/config.example.toml) to `~/.config/metricsd/config.toml`:

```toml
interval  = "5s"
retention = "7d"

[net]
exclude_interfaces = ["lo", "veth*", "br-*", "docker*"]
```

Ready-made configs for laptops, Docker hosts, Kubernetes nodes, long-term retention, and more live in [`examples/`](examples/).

## Documentation

| Doc                                           | Covers                                              |
| --------------------------------------------- | --------------------------------------------------- |
| [Installation](docs/installation.md)          | Requirements, `make install`, manual install        |
| [Uninstall](docs/uninstall.md)                | Removing binaries, service, and data                |
| [Usage](docs/usage.md)                        | Every `metricsctl` command and flag                 |
| [Configuration](docs/configuration.md)        | Every config key, defaults, validation              |
| [Metrics](docs/metrics.md)                    | Metric names, labels, units, sources                |
| [API](docs/api.md)                            | Endpoints, parameters, JSON schemas, errors         |
| [systemd](docs/systemd.md)                    | Running as a service, sandboxing, drop-ins, logs    |
| [Architecture](docs/architecture.md)          | How it works, storage schema, disk usage sizing     |
| [Troubleshooting](docs/troubleshooting.md)    | Common errors and fixes                             |
| [Examples](examples/README.md)                | Annotated example configs                           |

## Project layout

```
cmd/metricsd/        daemon entry point
cmd/metricsctl/      CLI client
internal/collector/  one collector per metric group
internal/store/      SQLite storage and retention
internal/api/        HTTP handlers and Unix socket listener
internal/config/     TOML config, defaults, validation
internal/wire/       JSON types shared by daemon and client
deploy/              systemd unit and reference config
examples/            example configs with explanations
docs/                user documentation
```

## License

[MIT](LICENSE) © 2026 Brian Nunez
