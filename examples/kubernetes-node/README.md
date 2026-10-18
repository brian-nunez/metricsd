# kubernetes-node

For Kubernetes or k3s nodes. Pod networking creates many short-lived interfaces; this config filters them out so only the node's real network links are recorded.

## Settings

| Key | Value | Why |
| --- | ----- | --- |
| `interval` | `15s` | A common monitoring cadence for servers; a third of the default's storage. |
| `retention` | `14d` | Two weeks covers a typical release cycle for before/after comparisons. |
| `[net] exclude_interfaces` | CNI patterns | See below. |
| `[diskio] exclude_devices` | adds `nbd*`, `zram*` | Network block devices (Longhorn, Ceph RBD clients) and compressed swap mirror other devices' I/O or aren't real disks. |

### Interfaces by CNI

| CNI | Interfaces created | Pattern |
| --- | ------------------ | ------- |
| Flannel (k3s default) | `flannel.1`, `cni0`, `veth*` | `flannel*`, `cni*`, `veth*` |
| Calico | `cali*`, `tunl0`, `vxlan.calico` | `cali*`, `tunl*`, `vxlan*` |
| Cilium | `lxc*`, `cilium_host`, `cilium_net`, `cilium_vxlan` | `lxc*`, `cilium_*` |
| kube-proxy IPVS / kube-router | `kube-ipvs0`, `kube-bridge` | `kube-*` |
| NodeLocal DNSCache | `nodelocaldns` | `nodelocaldns` |

The list covers all of them, so it works whichever CNI the cluster uses. Patterns that match nothing cost nothing.

## Footprint

About 50 series → **~10 MiB/day, ~140 MiB at full retention**.

## Running on a node

Nodes are usually headless, so enable lingering for the account that runs metricsd:

```bash
loginctl enable-linger "$USER"
systemctl --user enable --now metricsd
```

Check which interfaces remain after filtering:

```bash
metricsctl list
metricsctl now | grep '^net\.'
```

## Scope

metricsd measures the **node**. For per-pod and per-container metrics, use the kubelet's cAdvisor endpoint or metrics-server. metricsd's value on a node is a dependency-free view that keeps working when the cluster's own monitoring is down.
