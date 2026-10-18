# Containers: Docker, Podman, Kubernetes

metricsd measures **the environment it runs in**. On a server it measures the server; in a VM, the VM; in a container, that container. Install it at each level and you get independent sources:

| metricsd runs in | Reports |
| ---------------- | ------- |
| Physical server (128 GB, 64 cores) | 128 GB, 64 cores |
| VM on that server (16 GB, 4 vCPU) | 16 GB, 4 vCPU |
| Docker container in that VM (`--memory=1g --cpus=1`) | 1 GB, 1 CPU |

The central server receives three sources and links them however it likes.

## Why containers are different

A VM has its own kernel, so `/proc` inside it describes the VM. A container **shares its host's kernel**, and most of `/proc` still describes the host. In a container limited to 256 MB and 1 CPU on a 16 GB, 12-thread machine:

| Question | `/proc` says | The container's real limit |
| -------- | ------------ | -------------------------- |
| RAM | 16 GB (`/proc/meminfo`) | 256 MB (`memory.max`) |
| CPUs | 12 (`/proc/stat`) | 1 (`cpu.max`) |

So in a container, metricsd reads the container's **cgroup** (`/sys/fs/cgroup`): the same files Docker and Podman use to enforce limits and count usage. Network (`/proc/net/*`) and process lists already belong to the container's own namespaces and are read as usual.

## Detection

At startup metricsd checks for markers the runtimes leave inside containers, in this order:

| Marker | Environment |
| ------ | ----------- |
| `KUBERNETES_SERVICE_HOST` set, or `/var/run/secrets/kubernetes.io` exists | `kubernetes` |
| `/run/.containerenv` exists, or `container=podman` | `podman` |
| `/.dockerenv` exists | `docker` |
| `container` environment variable set (LXC, systemd-nspawn, ...) | `container` |
| none of the above | `vm` or `physical` |

The markers are identical for rootful and rootless runtimes. Override detection when needed:

```toml
environment = "docker"    # auto | physical | vm | docker | podman | kubernetes | container
```

The result is logged at startup (`environment=docker`) and shown by `metricsctl info`.

## What a container reports

| Metric group | Source in a container |
| ------------ | --------------------- |
| `cpu.*` | `cpu.stat`, `cpu.max`: usage as % of the container's CPU allowance, `cpu.cores` = its quota (or the CPUs it may run on), throttling |
| `mem.*`, `swap.*` | `memory.current`, `memory.max`, `memory.stat`, `memory.events`: used excludes reclaimable page cache, like `docker stats`; OOM kills |
| `diskio.*` | `io.stat`: this container's own reads and writes |
| `pressure.*` | `cpu.pressure`, `memory.pressure`, `io.pressure` |
| `procs.*`, `uptime.seconds` | PID namespace and `pids.current`/`pids.max`; uptime = age of PID 1 |
| `disk.*` | The container's `/` (overlay or fuse-overlayfs) and mounted volumes; single-file mounts like `/etc/hosts` are skipped |
| `net.*`, `tcp.*`, `udp.*`, `sockets.used` | The container's network namespace |

Without a memory or CPU limit, the host's capacity is reported as the ceiling, because that is what the container can actually use.

**Not reported in containers:** `load.*`, `temp.*`, `fan.*`, `battery.*`, `power.*`. Load averages are machine-wide kernel figures, and sensors belong to the host's hardware; inside a container they would describe the host. Measure those with metricsd on the host itself.

Full list: [metrics.md](metrics.md).

## Adding metricsd to an image

metricsd runs **inside** the container it measures, next to your application. Build the metricsd image once ([installation.md](installation.md#docker-image)) and copy its binaries into your image:

```dockerfile
FROM alpine:3.24
COPY --from=metricsd:latest /usr/local/bin/ /usr/local/bin/
COPY myapp /usr/local/bin/myapp
CMD ["sh", "-c", "metricsd & exec myapp"]
```

The binaries are static, so any base image works, including `scratch` and distroless. For production, use a small init such as `tini` or `s6-overlay` so both processes get signals and zombie reaping:

```dockerfile
RUN apk add --no-cache tini
ENTRYPOINT ["/sbin/tini", "--"]
CMD ["sh", "-c", "metricsd & exec myapp"]
```

Running the metricsd image by itself measures only that image's own container, which is rarely useful.

Ready-made setups: [examples/docker-container/](../examples/docker-container/), [examples/podman-rootless/](../examples/podman-rootless/), [examples/kubernetes-pod/](../examples/kubernetes-pod/).

## Reading metrics

The API socket lives inside the container, so read it from there:

```bash
docker exec myapp metricsctl now
docker exec myapp metricsctl info
podman exec myapp metricsctl history mem.used_bytes -since 1h
```

To send metrics to a central server, enable [OTLP export](export.md).

### Default paths in containers

| | Root in the container | Non-root user |
| - | -------------------- | ------------- |
| Database | `/var/lib/metricsd/metrics.db` | `/tmp/metricsd/metrics.db` |
| Socket | `/tmp/metricsd/metricsd.sock` | `/tmp/metricsd/metricsd.sock` |

The container's filesystem is discarded with the container. Mount a volume at the database directory to keep history (and the export backlog) across container replacements.

**Read-only root filesystem** (`--read-only`, `readOnlyRootFilesystem: true`): mount a writable tmpfs at `/tmp` (`--tmpfs /tmp`) or point `db_path` and `socket_path` at a writable volume.

## Identity

| Environment | `id` (and `metricsd.id` in exports) |
| ----------- | ----------------------------------- |
| Docker, Podman | The full container ID, read from `/proc/self/mountinfo` or the cgroup path; hostname as fallback |
| Kubernetes | `<pod-uid>/<container-name>` from the kubelet's mounts; container ID or hostname as fallback |

For Kubernetes, expose pod details through the downward API so they are exact:

```yaml
env:
  - name: POD_NAME
    valueFrom: { fieldRef: { fieldPath: metadata.name } }
  - name: POD_NAMESPACE
    valueFrom: { fieldRef: { fieldPath: metadata.namespace } }
  - name: NODE_NAME
    valueFrom: { fieldRef: { fieldPath: spec.nodeName } }
```

Without them, the namespace comes from the service account token mount and the pod name from the hostname.

## Rootless Docker and Podman

Rootless runtimes are supported. Two things differ from rootful setups.

### Controller delegation

A rootless container only gets the cgroup controllers systemd delegates to your user. On many distributions that is `memory` and `pids` only. metricsd treats every cgroup file as optional; with partial delegation:

| Delegated | Effect |
| --------- | ------ |
| `memory` | `mem.*`, `swap.*`, OOM kills available |
| `pids` | `procs.threads`, `procs.limit` available |
| no `cpu` | `cpu.usage_pct` relative to the CPUs the container can see; no `cpu.max` quota, no throttling metrics |
| no `io` | no `diskio.*` (logged once: `collector skipped ... io controller not delegated`) |

Missing data is left out, never reported as zero. To delegate everything, as root:

```bash
sudo mkdir -p /etc/systemd/system/user@.service.d
sudo tee /etc/systemd/system/user@.service.d/delegate.conf <<'EOF'
[Service]
Delegate=cpu cpuset io memory pids
EOF
sudo systemctl daemon-reload
```

Log out and back in (or reboot), then recreate the containers. Podman's and Docker's rootless guides recommend the same file.

### Non-root users

Rootless runtimes often run the container process as a non-root user (for example with `--userns=keep-id` or `USER` in the image). metricsd then uses `/tmp/metricsd` for its database and socket; see [Default paths](#default-paths-in-containers).

## Requirements and limits

- **cgroup v2** on the host. It's the default on every current distribution (systemd 247+, Docker 20.10+). On cgroup v1 hosts container CPU, memory, disk IO, and pressure metrics are unavailable and metricsd logs why at startup.
- Docker and Podman use a private cgroup namespace by default on cgroup v2; `--cgroupns=host` also works.

## Verification

Results from testing on a cgroup v2 host with rootful Docker 29:

| Scenario | Result |
| -------- | ------ |
| `--memory=1g --cpus=1`, CPU and memory load | `cpu.cores` 1, `mem.total_bytes` 1.0 GiB; CPU 99.9% vs `docker stats` 98–99%; memory 9–11 MiB vs 11.5 MiB |
| `--cgroupns=host --cpus=0.5` | `cpu.cores` 0.5; container ID read from the cgroup path |
| `--user 1000:1000 --memory=512m` | 512 MiB reported; database in `/tmp/metricsd` |
| Simulated rootless delegation (memory + pids only, no `cpu.max`, `io.stat`, PSI) | diskio and pressure skipped with a log line; memory limit, OOM kills, threads correct; no fabricated zeros |
| `/run/.containerenv` present | detected as `podman` |
| `KUBERNETES_SERVICE_HOST` set | detected as `kubernetes`; namespace and node from env |

Rootless Podman and rootless Docker were not run end to end on the test machine (Podman isn't installed there). Their behaviour was covered by the simulated delegation and non-root runs above, which exercise the same code paths.
