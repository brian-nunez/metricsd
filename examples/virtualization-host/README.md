# virtualization-host

For machines that run virtual machines (libvirt/KVM, virt-manager, GNOME Boxes, Proxmox). Each VM adds tap interfaces and often a block device per disk. This config records the host's physical links and disks and drops the per-VM ones.

## Settings

### Network

| Pattern | Created by | Excluded? |
| ------- | ---------- | --------- |
| `vnet*` | libvirt, one tap per VM NIC | yes |
| `tap*` | Proxmox / raw QEMU | yes |
| `fwbr*`, `fwpr*`, `fwln*` | Proxmox per-VM firewall plumbing | yes |
| `macvtap*` | VMs attached directly to a NIC | yes |
| `veth*` | LXC containers | yes |
| `virbr0` | libvirt default NAT network | **no** – total traffic of NATed guests |
| `vmbr*` | Proxmox bridges | **no** – usually carries all guest traffic to the LAN |

### Disk IO

| Pattern | What it is |
| ------- | ---------- |
| `dm-*` | LVM logical volumes, including LVM-thin guest disks |
| `zd*` | ZFS zvols used as guest disks |
| `nbd*` | Network block devices (qemu-nbd) |

Excluding these means disk IO reflects the physical drives (`sda`, `nvme0n1`), which carry all guest IO combined.

## Footprint

Host interfaces and disks only → about the default **~98 MiB at full retention** (1 day), no matter how many VMs run.

## Useful queries

```bash
# Combined guest disk load on the physical NVMe
metricsctl history diskio.write_bytes_per_sec -since 6h -labels device=nvme0n1

# NAT guest traffic
metricsctl history net.rx_bytes_per_sec -since 6h -labels iface=virbr0
```

## Variations

- **Watch one VM's traffic:** find its tap with `virsh domiflist <vm>` and exclude the other taps by explicit name instead of `vnet*`. Tap names change when VMs restart, so this suits long-running VMs.
- **Proxmox nodes** run as root by convention. metricsd works the same as a user service under any account; enable lingering for it.
