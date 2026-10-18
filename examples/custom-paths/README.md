# custom-paths

Moves the database to another disk, for example a larger data drive. This keeps writes off your system SSD and leaves room for longer retention.

Because the systemd unit sandboxes the daemon, a custom path also needs a one-time drop-in that grants write access.

## Settings

| Key | Value | Why |
| --- | ----- | --- |
| `db_path` | `/mnt/data/metricsd/metrics.db` | Database on the data disk. The `-wal` and `-shm` files go beside it. |
| `retention` | `30d` | A month of history: ~2.9 GiB at the default interval for a typical desktop (169 series). |

## Setup

1. **Create the directory** (systemd won't create it for you outside its own directories):
   ```bash
   mkdir -p /mnt/data/metricsd
   chmod 700 /mnt/data/metricsd
   ```

2. **Install the config:**
   ```bash
   cp examples/custom-paths/config.toml ~/.config/metricsd/config.toml
   ```

3. **Grant the sandbox write access:**
   ```bash
   systemctl --user edit metricsd
   ```
   Add:
   ```ini
   [Service]
   ReadWritePaths=/mnt/data/metricsd
   ```

4. **Restart and confirm:**
   ```bash
   systemctl --user restart metricsd
   journalctl --user -u metricsd -n 3      # db=/mnt/data/metricsd/metrics.db
   ls -l /mnt/data/metricsd/
   ```

### If the disk mounts late

If `/mnt/data` is a removable or network-mounted disk, have systemd wait for the mount instead of failing. Add this to the same drop-in:

```ini
[Unit]
RequiresMountsFor=/mnt/data/metricsd
```

## Moving existing data

```bash
systemctl --user stop metricsd
mv ~/.local/state/metricsd/metrics.db* /mnt/data/metricsd/
systemctl --user start metricsd
```

Move all three files (`metrics.db`, `metrics.db-wal`, `metrics.db-shm`) together, with the daemon stopped.

## Custom socket path

`socket_path` can be changed too, but keep these rules in mind:

| Rule | Reason |
| ---- | ------ |
| At most 107 bytes | Kernel limit for Unix socket paths |
| Not under `/tmp` | The service has a private `/tmp` (`PrivateTmp=yes`), so clients couldn't see the socket |
| Directory needs `ReadWritePaths` | Same sandbox rule as the database |
| Pass it to every client | `metricsctl -socket /your/path now` |

The default (`$XDG_RUNTIME_DIR/metricsd/metricsd.sock`) is the right choice almost always.

## Troubleshooting

| Symptom | Fix |
| ------- | --- |
| `status=226/NAMESPACE` | The `ReadWritePaths` directory doesn't exist. Create it (step 1). |
| `create data dir: ... read-only file system` | The drop-in is missing or names a different directory. |
| `unable to open database file` | Directory permissions: it must be owned by your user. |
