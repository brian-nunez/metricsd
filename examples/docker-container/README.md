# docker-container

Runs metricsd inside your application's container, so it measures **that container**: its CPU limit, its memory limit, its own disk and network IO. A container limited to 1 GiB and 1 CPU reports `mem.total_bytes = 1 GiB` and `cpu.cores = 1`, regardless of how big the host is.

Works the same with rootful and rootless Docker. For rootless setups, see also [podman-rootless](../podman-rootless/) for cgroup delegation.

## Settings

| Key | Value | Why |
| --- | ----- | --- |
| `interval` | `10s` | Containers are numerous; half the default storage per container. |
| `retention` | `1d` | Container storage is usually thrown away on redeploy. Ship long-term data to a central server with [OTLP export](../otlp-http/). |
| `[tags] service` | `my-app` | Names the workload in `metricsctl info` and in exported data. |

`environment`, `db_path`, and `socket_path` are left out on purpose: metricsd detects Docker from `/.dockerenv` and picks container-friendly paths (`/var/lib/metricsd` as root, `/tmp/metricsd` otherwise).

## Add metricsd to your image

Build the metricsd image once from the repository root:

```bash
docker build -t metricsd:latest .
```

Then copy the binaries into your application image and start metricsd alongside your app:

```dockerfile
FROM node:22-alpine
WORKDIR /app
COPY . .

COPY --from=metricsd:latest /usr/local/bin/metricsd /usr/local/bin/metricsctl /usr/local/bin/
COPY examples/docker-container/config.toml /etc/metricsd/config.toml

# metricsd runs in the background; your app stays PID 1's child via exec,
# so `docker stop` signals reach it.
CMD ["sh", "-c", "metricsd -config /etc/metricsd/config.toml & exec node server.js"]
```

The binaries are static, so they work in any base image: Alpine, Debian, distroless (use an entrypoint script there), or `scratch`.

When your app exits, the container stops and metricsd with it. If metricsd should never outlive or be orphaned by the app, use a tiny init such as `tini` (`docker run --init`) to reap it.

## Run with limits

```bash
docker run -d --name my-app --memory=1g --cpus=1 my-app:latest
```

Without `--memory` or `--cpus`, the container can use the whole host, and metricsd reports the host's capacity as the container's ceiling.

## Read the metrics

```bash
docker exec my-app metricsctl info
docker exec my-app metricsctl now
docker exec my-app metricsctl history cpu.usage_pct -since 30m
```

`metricsctl info` should show `Environment docker` and the container's full ID. `metricsctl now` shows:

| Metric | Inside the container means |
| ------ | -------------------------- |
| `cpu.cores` | The CPU limit (`--cpus`), or the CPUs the container may run on |
| `cpu.usage_pct{core=all}` | Usage as a share of that limit; 100% = using the whole allowance |
| `cpu.throttled_periods_pct`, `cpu.throttled_time_pct` | How often the limit held the container back |
| `mem.total_bytes` | The memory limit (`--memory`) |
| `mem.used_bytes` | Usage excluding reclaimable page cache (matches `docker stats`) |
| `mem.oom_kills` | Processes killed for exceeding the limit |
| `procs.count`, `procs.threads`, `procs.limit` | Processes, tasks, and the PID limit |
| `disk.*{mount=/}` | The container's root filesystem, plus mounted volumes |

Load averages and hardware sensors are machine-wide, so they're not reported inside containers.

## Ship metrics out

Append an export section so each container reports to your central server:

```toml
[export.otlp]
enabled  = true
protocol = "http"
endpoint = "http://otel-collector:4318"
```

Or leave `endpoint` empty and set `OTEL_EXPORTER_OTLP_ENDPOINT` with `docker run -e`. Each container is exported with `container.id`, `container.runtime.name = docker`, and `metricsd.environment = docker` resource attributes.

## Persist history (optional)

```bash
docker run -d -v my-app-metrics:/var/lib/metricsd --memory=1g --cpus=1 my-app:latest
```

More: [docs/containers.md](../../docs/containers.md).
