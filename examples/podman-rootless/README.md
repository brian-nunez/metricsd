# podman-rootless

metricsd inside a container started by **rootless** Podman, measuring that container. The same applies to rootless Docker.

Rootless runtimes can only use the cgroup controllers systemd delegates to your user. Most distributions delegate only `memory` and `pids` by default, which limits what the container can measure until you delegate the rest.

## Settings

| Key | Value | Why |
| --- | ----- | --- |
| `interval` | `10s` | Half the default storage per container. |
| `retention` | `1d` | Container storage is usually ephemeral. |
| `[tags]` | `service`, `runtime_mode` | Identify the workload. Tags are free-form; `runtime_mode` is just a label for your own filtering. |

metricsd detects Podman from `/run/.containerenv` (present in rootful and rootless containers) and reports `Environment podman`.

## What you get without delegation

| Metric group | Without `cpu`/`io` delegation | With full delegation |
| ------------ | ----------------------------- | -------------------- |
| `cpu.usage_pct`, `cpu.mode_pct` | ✓ (usage is always accounted) | ✓ |
| `cpu.cores` | CPUs visible to the container, **not** the `--cpus` limit | The `--cpus` limit |
| `cpu.throttled_*` | – | ✓ |
| `mem.*`, `swap.*`, `mem.oom_kills` | ✓ | ✓ |
| `diskio.*` | – (skipped: "io controller not delegated") | ✓ |
| `pressure.*` | depends on kernel; often – | ✓ |
| `procs.*`, `disk.*`, `net.*`, `tcp.*`, `udp.*`, `uptime.seconds` | ✓ | ✓ |

Without delegation, Podman also cannot enforce `--cpus` at all, so `cpu.cores` correctly reports what the container really may use.

Skipped collectors are logged once at startup:

```
level=INFO msg="collector skipped" collector=diskio reason="io controller not delegated to /sys/fs/cgroup"
```

## Enable full delegation

Check what your user gets:

```bash
cat "/sys/fs/cgroup/user.slice/user-$(id -u).slice/user@$(id -u).service/cgroup.controllers"
# memory pids              <- default on many distributions
```

Delegate the rest (requires root once):

```bash
sudo mkdir -p /etc/systemd/system/user@.service.d
sudo tee /etc/systemd/system/user@.service.d/delegate.conf <<'EOF'
[Service]
Delegate=cpu cpuset io memory pids
EOF
sudo systemctl daemon-reload
```

Log out and back in (or reboot), then re-check: the file should list `cpuset cpu io memory pids`.

## Run

Build an image containing metricsd as in [docker-container](../docker-container/), then:

```bash
podman run -d --name my-app --memory=1g --cpus=1 my-app:latest
podman exec my-app metricsctl info
podman exec my-app metricsctl now
```

## Notes for rootless

- **User inside the container.** Rootless containers usually run as UID 0 inside (mapped to your user outside), so the database goes to `/var/lib/metricsd`. With `--userns=keep-id` or a `USER` directive, metricsd runs unprivileged and uses `/tmp/metricsd`.
- **Root filesystem.** Rootless storage often uses `fuse-overlayfs`; `disk.*{mount=/}` reports it like any overlay.
- **Container ID.** Read from the storage path (`~/.local/share/containers/storage/overlay-containers/<id>`), so `metricsctl info` shows the same ID as `podman ps --no-trunc`.
- **cgroup v1** is not supported for container metrics. Rootless Podman needs cgroup v2 for limits anyway.

More: [docs/containers.md](../../docs/containers.md).
