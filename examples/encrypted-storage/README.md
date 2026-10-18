# encrypted-storage

For systems using LUKS encryption or LVM. These create device-mapper devices (`dm-0`, `dm-1`, ...) layered on top of the physical disk. Every byte written to `dm-0` is also written to the disk underneath, so recording both double counts.

The default config records the **physical disk** and skips `dm-*`. This example does the opposite.

## Which one do you want?

| Record | Config | Good for |
| ------ | ------ | -------- |
| Physical disk (`nvme0n1`) | default | Total load on the hardware, including LUKS overhead and every volume combined |
| Logical devices (`dm-*`) | this example | Load per LVM volume, e.g. separating `/home` from `/var` |

With a single LUKS volume and no LVM, both views show the same numbers, and the default is simpler.

## Settings

| Pattern | Excludes |
| ------- | -------- |
| `nvme*` | NVMe drives |
| `sd*`   | SATA/SAS/USB drives |
| `vd*`   | virtio disks (VMs) |
| `loop*`, `ram*`, `zram*` | Not real disks |

`dm-*` is absent from the list, so device-mapper devices are recorded.

## Mapping dm-N to names

Labels use the kernel name (`device=dm-0`). To see what each one is:

```bash
ls -l /dev/mapper/          # root -> ../dm-0, home -> ../dm-1
lsblk -o NAME,KNAME,TYPE,MOUNTPOINTS
```

Kernel names follow creation order at boot, so they're stable as long as your volume layout doesn't change.

## Footprint

Same as the default (one or a few devices either way): **~32 MiB/day, ~220 MiB at full retention**.

## Useful queries

```bash
# Write load on the decrypted root volume
metricsctl history diskio.write_bytes_per_sec -since 1h -labels device=dm-0
```

## Variations

- **Both views at once:** set `exclude_devices = ["loop*", "ram*", "zram*"]`. Expect the totals to double count.
- **Only one physical disk is encrypted:** list just that disk, e.g. `["loop*", "ram*", "zram*", "nvme0n1"]`, so other disks are still recorded.
