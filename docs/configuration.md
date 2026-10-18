# Configuration

metricsd works without a config file. To customize it, create a [TOML](https://toml.io) file at:

```
~/.config/metricsd/config.toml        # or $XDG_CONFIG_HOME/metricsd/config.toml
```

A fully commented reference with every default is in [`deploy/config.example.toml`](../deploy/config.example.toml). Ready-made configs for common setups are in [`examples/`](../examples/README.md).

Apply changes by restarting:

```bash
systemctl --user restart metricsd
```

## How the file is loaded

- **Defaults first.** Every key is optional; anything you leave out keeps its default.
- **Lists replace, maps merge.** Setting `exclude_interfaces` replaces the whole default list, so include the defaults you still want. Setting one entry under `[collectors]` changes only that collector.
- **Unknown keys are errors.** A typo such as `intreval` stops the daemon with `unknown keys: [intreval]` instead of being silently ignored.
- **Missing file is fine** at the default path. A missing file passed with `-config` is an error.

```bash
metricsd -config /path/to/config.toml
```

## Reference

### Top-level keys

| Key           | Type     | Default | Description |
| ------------- | -------- | ------- | ----------- |
| `interval`    | duration | `"5s"`  | Time between samples. Minimum `1s`. |
| `retention`   | duration | `"7d"`  | How long samples are kept. Must be at least `interval`. Old samples are pruned at startup and then hourly. |
| `db_path`     | path     | `~/.local/state/metricsd/metrics.db` | SQLite database file. Parent directory is created if needed (mode `0700`). |
| `socket_path` | path     | `$XDG_RUNTIME_DIR/metricsd/metricsd.sock` | Unix socket for the API. At most 107 bytes. |

### `[collectors]`

Each collector can be switched on or off. All are on by default.

| Key      | Metrics produced |
| -------- | ---------------- |
| `cpu`    | `cpu.usage_pct` |
| `mem`    | `mem.*`, `swap.*` |
| `load`   | `load.1m`, `load.5m`, `load.15m`, `uptime.seconds` |
| `disk`   | `disk.total_bytes`, `disk.used_bytes`, `disk.free_bytes`, `disk.used_pct` |
| `diskio` | `diskio.read_bytes_per_sec`, `diskio.write_bytes_per_sec` |
| `net`    | `net.rx_bytes_per_sec`, `net.tx_bytes_per_sec` |
| `temp`   | `temp.celsius` |

```toml
[collectors]
temp = false     # e.g. in a VM with no sensors
```

Disabling a collector stops new samples. Its existing history stays queryable until retention removes it.

### `[net]`

| Key                  | Type          | Default                                | Description |
| -------------------- | ------------- | -------------------------------------- | ----------- |
| `exclude_interfaces` | list of globs | `["lo", "veth*", "br-*", "docker*"]`   | Interfaces to skip. Names come from `/proc/net/dev` (or `ip -br link`). |

### `[diskio]`

| Key               | Type          | Default                       | Description |
| ----------------- | ------------- | ----------------------------- | ----------- |
| `exclude_devices` | list of globs | `["loop*", "ram*", "dm-*"]`   | Block devices to skip. Only whole devices (entries in `/sys/block`) are ever reported; partitions are always skipped. |

`dm-*` is excluded by default because device-mapper devices (LUKS, LVM) duplicate the I/O of the disk underneath them. See the [encrypted-storage example](../examples/encrypted-storage/) to track them instead.

## Value formats

### Durations

| Form            | Examples                      |
| --------------- | ----------------------------- |
| Go duration     | `"500ms"`, `"30s"`, `"5m"`, `"12h"`, `"1h30m"` |
| Whole/fractional days | `"7d"`, `"90d"`, `"1.5d"` |

Mixed day forms such as `"1d12h"` are not supported; write `"36h"`.

### Paths

- Must be absolute.
- A leading `~/` expands to your home directory.
- Environment variables are **not** expanded (`$HOME/x` is rejected as relative).

### Globs

Shell-style patterns matched against the whole name:

| Pattern  | Matches |
| -------- | ------- |
| `*`      | any run of characters |
| `?`      | one character |
| `[abc]`, `[a-z]` | one character from the set |
| `\*`     | a literal `*` |

`veth*` matches `veth2649d83`; `wl*` matches every Wi-Fi interface; `nvme?n1` matches `nvme0n1` and `nvme1n1`. Invalid patterns (for example an unclosed `[`) are rejected at startup.

## Validation rules

The daemon refuses to start, logging the reason, when:

| Rule | Example error |
| ---- | ------------- |
| `interval` < 1s | `interval must be at least 1s, got 500ms` |
| `retention` < `interval` | `retention (1s) must be at least the interval (5s)` |
| Relative or empty path | `db_path must be an absolute path, got "data/metrics.db"` |
| Socket path > 107 bytes | `socket_path is 120 bytes; Unix sockets allow at most 107` |
| Unknown collector | `unknown collector "gpu" (valid: cpu, mem, load, disk, diskio, net, temp)` |
| Bad glob | `invalid exclude pattern "[eth": syntax error in pattern` |
| Unknown key | `unknown keys: [intreval]` |

Check the reason with `journalctl --user -u metricsd -n 20`.

## Changing paths under systemd

The service's sandbox only allows writes to `~/.local/state/metricsd` and `$XDG_RUNTIME_DIR/metricsd`. A custom `db_path` or `socket_path` elsewhere needs a drop-in granting write access. The [custom-paths example](../examples/custom-paths/) walks through it, and [systemd.md](systemd.md#drop-ins) explains drop-ins.

If you change `socket_path`, pass the same path to `metricsctl -socket`.

## Choosing interval and retention

Disk usage grows with series × samples kept. At about 35 bytes per point:

```
MiB per day ≈ series × (86400 / interval_seconds) × 35 / 1048576
```

A typical desktop reports about 55 series. See [architecture.md](architecture.md#disk-usage) for a sizing table.
