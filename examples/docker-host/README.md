# docker-host

For machines running Docker. Records the host's real interfaces plus Docker's default bridge, while skipping the per-container interfaces that appear and disappear with every container.

## Settings

| Pattern | Excluded? | What it is |
| ------- | --------- | ---------- |
| `lo` | yes | Loopback |
| `veth*` | yes | One virtual Ethernet pair per running container. Names are random and change on every restart, so each restart creates a new series. |
| `br-*` | yes | One bridge per user-defined network (every `docker compose` project gets one). |
| `docker0` | **no** | Docker's default bridge. Its traffic is the total for containers on the default network, which is a useful single "container traffic" number. |

The default config excludes `docker*` as well. This example keeps `docker0` on purpose.

## Why per-container interfaces are a problem

Each container restart produces a new `veth` name, and therefore a new series that records for a while and then stops. Over a week of deploys, that leaves hundreds of short-lived series in `metricsctl now` and `list`. They're pruned once they age past `retention`, but they're noise until then.

## Footprint

Default settings plus eight `docker0` series → **~103 MiB/day, ~720 MiB at full retention** for a typical desktop.

## Useful queries

```bash
# Container traffic on the default bridge
metricsctl history net.tx_bytes_per_sec -since 6h -labels iface=docker0

# Host uplink for comparison (interface from `ip -br link`)
metricsctl history net.tx_bytes_per_sec -since 6h -labels iface=enp3s0
```

## Variations

- **Compose-heavy host:** your containers live on `br-*` networks, not `docker0`. To record one compose network, find its bridge with `docker network ls` and `ip -br link`, then exclude the others explicitly instead of using `br-*`.
- **Podman:** add `podman*` and `cni-podman*` to the list.
- **Overlay/Swarm:** add `docker_gwbridge` and `vx-*`.

This config measures the **host**. To measure a container's own CPU, memory, and IO against its limits, run metricsd inside that container as well; see [docker-container](../docker-container/). Each one reports as a separate source.
