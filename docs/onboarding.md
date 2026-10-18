# Onboarding

This guide gives you one production config and the install steps for each environment. It also shows how to manage metricsd and which metrics it collects.

## How it works

- **metricsd measures the environment that it runs in.** On a VM, it reports the VM. In a container, it reports the CPU and memory limits of that container, not the limits of the host.
- **Install metricsd in each environment that you want to monitor.** For example, a container that runs on a VM on a physical server gives three independent sources.
- **metricsd exports metrics over OTLP (HTTP or gRPC).** While the OTLP endpoint is not available, metricsd keeps the data in a local SQLite database. Thus no data is lost.

```
 physical server ──┐
 VM ───────────────┼── OTLP ──► your OTLP endpoint
 container ────────┘
```

## Before you start

Get this information about your OTLP endpoint:

| You need | Example | Put it in |
| -------- | ------- | --------- |
| OTLP endpoint URL | `https://otlp.example.com:4318` | `endpoint` |
| Protocol | `http` (port 4318) or `grpc` (port 4317) | `protocol` |
| Authentication token or header | `Bearer abc123...` | `[export.otlp.headers]` or `OTEL_EXPORTER_OTLP_HEADERS` |
| Private CA certificate, if the endpoint uses one | `/etc/metricsd/ca.pem` | `ca_file` |
| Tag conventions | `env`, `service`, `team`, `region` | `[tags]` |

## 1. The production config

One config file works without changes on VMs, physical machines, Docker, Podman, and Kubernetes. metricsd detects the environment and adapts to it. Replace each `REPLACE_ME` value.

```toml
# metricsd production config.
# This file works without changes on VMs, physical machines, and containers.
# Reference: docs/configuration.md

# Collect a sample every 15 seconds. The local data is only a buffer for
# export. 3 days of data is enough to cover a long endpoint outage.
interval  = "15s"
retention = "3d"

# Tags describe this source. metricsd sends them with each export, so you
# can group and filter sources. Tag keys must not start with
# service., host., os., container., k8s., or metricsd. These prefixes are
# reserved for the identity that metricsd detects.
[tags]
env     = "production"
service = "REPLACE_ME"     # Example: "checkout-api"
team    = "REPLACE_ME"     # Example: "payments"
region  = "REPLACE_ME"     # Example: "us-east-1"

# Skip the virtual interfaces that Docker, Kubernetes network plugins, and
# hypervisors create. A pattern that matches no interface has no cost.
[net]
exclude_interfaces = [
  "lo", "veth*", "br-*", "docker*",
  "cali*", "tunl*", "vxlan*", "flannel*", "cni*", "lxc*", "cilium_*", "kube-*",
  "vnet*", "tap*",
]

# Skip devices that copy the IO of another disk (LUKS and LVM layers), and
# devices that are not real disks (loop devices, compressed RAM swap, CD-ROM).
[diskio]
exclude_devices = ["loop*", "ram*", "zram*", "dm-*", "nbd*", "sr*"]

[export.otlp]
enabled     = true
protocol    = "http"                                  # Or "grpc".
endpoint    = "https://REPLACE_ME.example.com:4318"   # gRPC usually uses port 4317.
compression = "gzip"
timeout     = "10s"
interval    = "30s"
# ca_file   = "/etc/metricsd/ca.pem"                  # Only for a private CA.

# To supply the header through the environment, delete this table and set:
#   OTEL_EXPORTER_OTLP_HEADERS="authorization=Bearer%20<token>"
[export.otlp.headers]
authorization = "Bearer REPLACE_ME"
```

### Why these values

| Setting | Value | Reason |
| ------- | ----- | ------ |
| `interval` | `15s` | This is a standard monitoring interval. It uses one third of the storage and export volume of the default interval. |
| `retention` | `3d` | Locally, 3 days of data covers an outage over a weekend. Disk use is approximately **100 MiB** on a machine and **45 MiB** in a container. |
| `export.otlp.interval` | `30s` | Each request contains two collection cycles. |
| `compression` | `gzip` | Gzip makes export traffic much smaller. All OTLP servers support it. |
| `timeout` | `10s` | This limit applies to each request. metricsd keeps failed batches in the buffer and tries again. The wait between attempts increases up to 5 minutes. |

### Keep the token private

- If the token is in the config file, set the file permissions with `chmod 600`.
- Or remove `[export.otlp.headers]` and set `OTEL_EXPORTER_OTLP_HEADERS="authorization=Bearer%20<token>"`. Write each space as `%20`. This method is better for containers and Kubernetes secrets.
- If the config file contains a headers table, metricsd uses the table and ignores the variable.

### Check the config

At startup, metricsd checks the full file. If it finds an error (an unknown key, an incorrect URL, or a reserved tag), it stops and shows an exact message. Run metricsd in the foreground one time:

```bash
metricsd -config ./config.toml    # Press Ctrl+C after you see "metricsd started".
```

## 2. VMs and physical machines

metricsd runs as a **systemd user service**. It does not need root access, it runs in a sandbox, and it starts when the machine starts.

### Install

Requirements: Linux, Go 1.27 or later, `make`, and systemd.

```bash
git clone git@github.com:brian-nunez/metricsd.git
cd metricsd
make install                                   # Installs binaries to ~/.local/bin and the unit to ~/.config/systemd/user.

mkdir -p ~/.config/metricsd
$EDITOR ~/.config/metricsd/config.toml         # Paste the production config.
chmod 600 ~/.config/metricsd/config.toml

systemctl --user enable --now metricsd
loginctl enable-linger "$USER"                 # Keeps metricsd running without a login session, and starts it at boot.
```

Make sure that `~/.local/bin` is on your `PATH`.

**Dedicated account (optional).** To run metricsd under its own user account instead of your account, do these steps:

```bash
sudo useradd --system --create-home --shell /usr/sbin/nologin metricsd
sudo loginctl enable-linger metricsd
sudo machinectl shell metricsd@ /bin/bash      # Opens a full user session for systemctl --user (package: systemd-container).
# In that shell, do the install steps above.
```

### Check that it works

```bash
systemctl --user status metricsd               # Shows "active (running)".
metricsctl info                                # Shows the environment (physical or vm) and the ID that metricsd exports.
metricsctl now                                 # Shows the current values. Rates show after the second cycle.
journalctl --user -u metricsd -n 20            # Shows "otlp export enabled" and no "export failed" warnings.
```

### Manage

| Task | Command |
| ---- | ------- |
| Show the status | `systemctl --user status metricsd` |
| Follow the logs | `journalctl --user -u metricsd -f` |
| Apply a config change | `systemctl --user restart metricsd` |
| Stop or start | `systemctl --user stop metricsd` or `systemctl --user start metricsd` |
| Upgrade | `git pull && make install`. This command restarts the running service. |
| Uninstall | `make uninstall`. This command keeps the data and the config. See [uninstall.md](uninstall.md). |

Files:

- Config: `~/.config/metricsd/config.toml`
- Database: `~/.local/state/metricsd/metrics.db`
- Socket: `$XDG_RUNTIME_DIR/metricsd/metricsd.sock`

For more, see [installation.md](installation.md) and [systemd.md](systemd.md).

## 3. Docker

metricsd runs **inside the container of your application**, as a second process next to your application. It measures that container. The binaries are static, so they work in all base images.

### Build the metricsd image one time

```bash
git clone git@github.com:brian-nunez/metricsd.git
cd metricsd
docker build -t metricsd:latest .              # About 38 MB. The image contains only the two binaries.
```

If your application images build on a different machine, push this image to your registry. Example: `registry.example.com/metricsd:1.0`.

### Add metricsd to your application image

```dockerfile
FROM node:22-alpine
WORKDIR /app
COPY . .

COPY --from=metricsd:latest /usr/local/bin/metricsd /usr/local/bin/metricsctl /usr/local/bin/
COPY metricsd.toml /etc/metricsd/config.toml

# metricsd runs in the background. Your app replaces the shell, so your app
# receives the signal from docker stop directly.
CMD ["sh", "-c", "metricsd -config /etc/metricsd/config.toml & exec node server.js"]
```

`metricsd.toml` is the production config. If you supply the token at runtime, remove the headers table from this file.

Images without a shell (distroless, `scratch`) need a shell for the two-process `CMD`. Copy a static shell into the image, for example `COPY --from=busybox:musl /bin/sh /bin/sh`.

### Run

```bash
docker run -d --name checkout-api \
  --memory=1g --cpus=1 \
  -e OTEL_EXPORTER_OTLP_HEADERS="authorization=Bearer%20$OTLP_TOKEN" \
  -v checkout-api-metrics:/var/lib/metricsd \
  checkout-api:latest
```

| Flag | Reason |
| ---- | ------ |
| `--memory`, `--cpus` | metricsd reports these limits as the capacity of the container. Without them, metricsd reports the capacity of the host. |
| `-e OTEL_EXPORTER_OTLP_HEADERS` | Keeps the token out of the image. |
| `-v ...:/var/lib/metricsd` | Keeps the export buffer when the container restarts. This flag is optional. Without it, you lose unsent data when you remove the container. Non-root containers use `/tmp/metricsd` instead. |

### Check and manage

| Task | Command |
| ---- | ------- |
| Show the identity | `docker exec checkout-api metricsctl info`. It shows the environment `docker` and the full container ID. |
| Show the current values | `docker exec checkout-api metricsctl now` |
| Show the logs | `docker logs checkout-api`. metricsd writes its logs to stderr, together with the logs of your application. |
| Apply a config change | Build the image again. Or mount the config with `-v ./metricsd.toml:/etc/metricsd/config.toml:ro` and run `docker restart`. |
| Upgrade | Build `metricsd:latest` again. Then build your application image again and deploy it. |

**Read-only root filesystem** (`--read-only`): For a non-root container, add `--tmpfs /tmp`. For a root container, add a volume at `/var/lib/metricsd`.

For more, see [containers.md](containers.md) and [examples/docker-container](../examples/docker-container/).

## 4. Podman (rootful and rootless)

The steps are the same as for Docker: the same image, the same `COPY --from`, and the same flags. Use `podman` instead of `docker`.

```bash
podman build -t metricsd:latest .
podman run -d --name checkout-api --memory=1g --cpus=1 \
  -e OTEL_EXPORTER_OTLP_HEADERS="authorization=Bearer%20$OTLP_TOKEN" \
  checkout-api:latest
podman exec checkout-api metricsctl info       # Shows the environment: podman.
```

**Rootless: enable cgroup delegation one time on each host.** Most distributions give only the `memory` and `pids` controllers to users. Thus rootless containers cannot see or enforce CPU limits, and they cannot see disk IO. metricsd continues to run and skips the data that it cannot read. For complete metrics, do these steps:

```bash
sudo mkdir -p /etc/systemd/system/user@.service.d
sudo tee /etc/systemd/system/user@.service.d/delegate.conf <<'EOF'
[Service]
Delegate=cpu cpuset io memory pids
EOF
sudo systemctl daemon-reload                   # Then log out and log in again.
```

| Without delegation | With delegation |
| ------------------ | --------------- |
| CPU usage, memory, swap, OOM kills, processes, disk space, network | All of these, plus the CPU limit, throttling, disk IO, and pressure |

Rootless Docker operates the same way and needs the same delegation. For more, see [examples/podman-rootless](../examples/podman-rootless/).

## 5. Kubernetes

Add metricsd to each application container that you want to measure. A separate sidecar container measures only itself.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: metricsd-otlp
type: Opaque
stringData:
  headers: "authorization=Bearer%20REPLACE_ME"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: checkout-api-metricsd
data:
  config.toml: |
    interval  = "15s"
    retention = "1d"

    [tags]
    env     = "production"
    service = "checkout-api"
    team    = "REPLACE_ME"
    cluster = "REPLACE_ME"      # A pod cannot detect the name of its cluster.

    [export.otlp]
    enabled     = true
    protocol    = "grpc"
    endpoint    = "http://otel-collector.observability.svc.cluster.local:4317"   # REPLACE_ME
    compression = "gzip"
    interval    = "30s"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: checkout-api
spec:
  replicas: 3
  selector:
    matchLabels: { app: checkout-api }
  template:
    metadata:
      labels: { app: checkout-api }
    spec:
      containers:
        - name: app
          image: registry.example.com/checkout-api:1.4.0     # Built with COPY --from=metricsd.
          command: ["sh", "-c", "metricsd -config /etc/metricsd/config.toml & exec /app/server"]
          resources:
            requests: { cpu: 250m, memory: 256Mi }
            limits:   { cpu: "1", memory: 1Gi }               # metricsd reports these limits as the capacity.
          env:
            - name: POD_NAME
              valueFrom: { fieldRef: { fieldPath: metadata.name } }
            - name: POD_NAMESPACE
              valueFrom: { fieldRef: { fieldPath: metadata.namespace } }
            - name: NODE_NAME
              valueFrom: { fieldRef: { fieldPath: spec.nodeName } }
            - name: OTEL_EXPORTER_OTLP_HEADERS
              valueFrom: { secretKeyRef: { name: metricsd-otlp, key: headers } }
          volumeMounts:
            - { name: metricsd-config, mountPath: /etc/metricsd, readOnly: true }
      volumes:
        - name: metricsd-config
          configMap: { name: checkout-api-metricsd }
```

- `retention = "1d"`: Pod storage is temporary. The buffer only needs to cover short outages.
- `POD_NAME`, `POD_NAMESPACE`, and `NODE_NAME` make the pod identity exact in exports (`k8s.pod.name`, `k8s.namespace.name`, `k8s.node.name`).
- Read-only root filesystem: For a non-root container, mount an `emptyDir` at `/tmp`. For a root container, mount it at `/var/lib/metricsd`.

| Task | Command |
| ---- | ------- |
| Show the identity | `kubectl exec deploy/checkout-api -c app -- metricsctl info` |
| Show the current values | `kubectl exec deploy/checkout-api -c app -- metricsctl now` |
| Show the logs | `kubectl logs deploy/checkout-api -c app` |
| Apply a config change | Edit the ConfigMap. Then run `kubectl rollout restart deploy/checkout-api`. |
| Upgrade | Build the image again with the new metricsd. Then deploy it. |

To measure the **nodes**, install metricsd on each node as in [section 2](#2-vms-and-physical-machines). Use the [kubernetes-node example](../examples/kubernetes-node/). For more, see [examples/kubernetes-pod](../examples/kubernetes-pod/).

## 6. What it collects

metricsd collects approximately 90 metrics on a machine and 70 metrics in a container. It collects one sample at each `interval`.

| Group | Examples | Machine | Container |
| ----- | -------- | :-----: | :-------: |
| CPU | `cpu.cores`, `cpu.usage_pct` (total and per core), `cpu.mode_pct` (user, system, iowait, steal...), `cpu.frequency_mhz`, context switches, interrupts | ✓ | Usage compared to the CPU limit, throttling |
| Memory | `mem.total_bytes`, `mem.used_bytes`, `mem.used_pct`, cached, dirty, slab, page faults, `swap.*` | ✓ | Usage compared to the memory limit, OOM kills, peak |
| Load | `load.1m`, `load.5m`, `load.15m` | ✓ | – |
| Disk space | `disk.used_pct`, `disk.free_bytes`, inodes, for each mount | ✓ | Root filesystem and volumes |
| Disk IO | `diskio.read_bytes_per_sec`, IOPS, latency, `diskio.busy_pct` | ✓ | Its own reads and writes |
| Network | `net.rx_bytes_per_sec`, packets, errors, drops, for each interface | ✓ | Its own interfaces |
| TCP and UDP | `tcp.established`, retransmits, resets, `udp.*`, socket counts | ✓ | Its own network namespace |
| System | `uptime.seconds`, `procs.count`, threads, open files, logged-in users | ✓ | Its processes and PID limit |
| Pressure | `pressure.some_pct`, `pressure.full_pct` for CPU, memory, and IO | ✓ | ✓ |
| Hardware | `temp.celsius`, `fan.rpm`, `battery.charge_pct`, `power.ac_online` | ✓ if the hardware is present | – |

Each export also contains the identity of the source:

- `metricsd.environment`: `physical`, `vm`, `docker`, `podman`, or `kubernetes`
- `metricsd.id` and `host.name`
- `container.id` or `k8s.*`, where they apply
- Your tags

For the full list with labels and units, see [metrics.md](metrics.md).

## 7. Configuration at a glance

| Key | Default | Notes |
| --- | ------- | ----- |
| `environment` | `auto` | Overrides detection: `physical`, `vm`, `docker`, `podman`, `kubernetes`, `container` |
| `interval` | `5s` | The minimum is `1s`. |
| `retention` | `1d` | Also sets how far back export can send old data after an outage. |
| `db_path`, `socket_path` | Depends on the environment | Usually, do not set these. |
| `[tags]` | None | metricsd sends tags as export resource attributes. |
| `[collectors]` | All on | Turn off one group, for example `battery = false`. |
| `[net] exclude_interfaces` | `lo`, `veth*`, `br-*`, `docker*` | A list that you set replaces the default. |
| `[diskio] exclude_devices` | `loop*`, `ram*`, `dm-*` | A list that you set replaces the default. |
| `[export.otlp]` | Disabled | `protocol`, `endpoint`, `headers`, `compression`, `timeout`, `interval`, `ca_file`, `cert_file`, `key_file` |

For all keys, see [configuration.md](configuration.md). For export details, see [export.md](export.md). For more configs that are ready to use, see [examples/](../examples/README.md).

## 8. Check that data gets to the OTLP endpoint

1. `metricsctl info` shows the expected environment and an ID.
2. After one minute, the log shows `otlp export enabled` and no `export failed` warnings.
3. Where you view the exported data, filter by `service.name = metricsd` and `metricsd.id = <ID from step 1>`. Metrics such as `cpu.usage_pct` should arrive at each export interval.

If the OTLP endpoint is not available, the log shows `export failed; data is buffered locally`. When the endpoint is available again, the log shows `export recovered`. Then the old data arrives with its original timestamps.

## Troubleshooting quick reference

| Symptom | Fix |
| ------- | --- |
| `cannot reach metricsd at ...` | The service does not run. Check it with `systemctl --user status metricsd`. If you changed `socket_path`, add `-socket`. |
| metricsd exits with `load config: ...` | The message names the incorrect key or value. Fix it and restart metricsd. |
| `export failed` with `401` or `403` | The token is incorrect or missing. |
| `export failed` with `404` over HTTP | The endpoint path is incorrect. Leave the path empty to use `/v1/metrics`. |
| A container shows the CPUs or memory of the host | The container has no `--memory` or `--cpus` limit, or it is rootless without delegation. |
| `collector skipped ... not delegated` | The container is rootless. Enable delegation (section 4). |

For more, see [troubleshooting.md](troubleshooting.md).
