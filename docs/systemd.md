# Running under systemd

metricsd ships a **user** unit: it runs as you, needs no root, and is managed with `systemctl --user`.

## Everyday commands

| Task | Command |
| ---- | ------- |
| Start now and at login | `systemctl --user enable --now metricsd` |
| Stop | `systemctl --user stop metricsd` |
| Restart (apply config) | `systemctl --user restart metricsd` |
| Status | `systemctl --user status metricsd` |
| Follow logs | `journalctl --user -u metricsd -f` |
| Recent logs | `journalctl --user -u metricsd -n 50` |
| Disable autostart | `systemctl --user disable metricsd` |

## Running without a login session

User services start at login and stop when your last session ends. For continuous collection on servers or headless machines:

```bash
loginctl enable-linger "$USER"
```

With lingering, your user manager starts at boot and metricsd runs whether or not you're logged in.

## The unit

`deploy/metricsd.service` (installed to `~/.config/systemd/user/metricsd.service`):

| Setting | Purpose |
| ------- | ------- |
| `Type=exec` | systemd reports a failure if the binary can't be executed |
| `Restart=on-failure`, `RestartSec=5s` | Restarts after crashes or startup errors |
| `StateDirectory=metricsd` | Creates `~/.local/state/metricsd` and sets `$STATE_DIRECTORY`, which metricsd uses as the database location |
| `RuntimeDirectory=metricsd` | Creates `$XDG_RUNTIME_DIR/metricsd` for the socket; removed when the service stops |
| `WantedBy=default.target` | Starts with your user session when enabled |

## Sandboxing

The unit locks the daemon down to what it needs: reading `/proc` and `/sys`, writing its database, serving one Unix socket, and opening outbound connections for [OTLP export](export.md).

| Setting | Effect |
| ------- | ------ |
| `ProtectSystem=strict` | Entire filesystem read-only |
| `ProtectHome=read-only` | Home directory read-only (config remains readable) |
| `PrivateTmp=yes` | Private `/tmp` invisible to other processes |
| `PrivateDevices=yes` | No access to physical devices in `/dev` |
| `NoNewPrivileges=yes` | Can never gain privileges (setuid, capabilities) |
| `ProtectKernelTunables=yes` | `/proc/sys` and `/sys` tunables read-only |
| `ProtectKernelModules=yes` | Can't load kernel modules |
| `ProtectControlGroups=yes` | cgroup filesystem read-only |
| `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6` | Unix sockets for the local API; IPv4/IPv6 only so OTLP export can reach its server. No raw, netlink, or packet sockets. |
| `RestrictNamespaces=yes` | Can't create namespaces |
| `RestrictRealtime=yes` | No realtime scheduling |
| `LockPersonality=yes` | Can't change execution domain |
| `MemoryDenyWriteExecute=yes` | No writable-and-executable memory |
| `SystemCallArchitectures=native` | Native syscalls only |
| `SystemCallFilter=@system-service` | Allow-list of syscalls typical services need |
| `UMask=0077` | Files created are private to you |

Writable exceptions are only the `StateDirectory` and `RuntimeDirectory`. User-mode sandboxing relies on unprivileged user namespaces, which every mainstream distribution enables.

Review the exposure score:

```bash
systemd-analyze --user security metricsd
```

## Drop-ins

Change unit settings without editing the installed file (which `make install` overwrites) by using a drop-in:

```bash
systemctl --user edit metricsd
```

This opens an editor for `~/.config/systemd/user/metricsd.service.d/override.conf`. Save, then restart the service.

### Allow a custom database or socket location

Required when `db_path` or `socket_path` points outside the sandbox's writable directories:

```ini
[Service]
ReadWritePaths=/mnt/data/metricsd
```

- Create the directory first (`mkdir -p /mnt/data/metricsd`). systemd refuses to start the unit if a `ReadWritePaths` entry is missing; prefix it with `-` to make it optional.
- A socket under `/tmp` is not reachable by `metricsctl`, because `PrivateTmp` gives the service its own `/tmp`. Keep sockets under `$XDG_RUNTIME_DIR`.

See the [custom-paths example](../examples/custom-paths/) for a complete walk-through.

### Use a different config file

```ini
[Service]
ExecStart=
ExecStart=%h/.local/bin/metricsd -config %h/.config/metricsd/laptop.toml
```

The empty `ExecStart=` clears the original before setting the new one. `%h` is your home directory.

### Limit resources

metricsd typically uses 20–30 MB RSS (measured 29 MB with OTLP export enabled) and a fraction of a percent of CPU. To enforce a ceiling:

```ini
[Service]
MemoryMax=64M
CPUQuota=5%
Nice=10
IOSchedulingClass=idle
```

## Verify a unit change without installing it

Validate syntax:

```bash
sed "s|@BINDIR@|$PWD/bin|" deploy/metricsd.service > /tmp/metricsd.service
systemd-analyze --user verify /tmp/metricsd.service
```

Run the binary under the same sandbox as a throwaway transient unit. Properties go in a bash array because some values contain spaces (`RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6`). The unit's own `StateDirectory`/`RuntimeDirectory` are replaced with test ones, so the test never touches an installed instance's database or socket:

```bash
make build
mapfile -t LINES < <(sed -n '/^\[Service\]/,/^\[/{/^[A-Z][A-Za-z]*=/p}' deploy/metricsd.service \
  | grep -vE '^(ExecStart|Type|Restart|RestartSec|StateDirectory|RuntimeDirectory)=')
ARGS=(); for l in "${LINES[@]}"; do ARGS+=(-p "$l"); done

systemd-run --user --unit=metricsd-test -p Type=exec "${ARGS[@]}" \
  -p StateDirectory=metricsd-test -p RuntimeDirectory=metricsd-test \
  -p Environment=XDG_RUNTIME_DIR=/run/user/$(id -u)/metricsd-test \
  "$PWD/bin/metricsd"

metricsctl -socket /run/user/$(id -u)/metricsd-test/metricsd/metricsd.sock now
journalctl --user -u metricsd-test -n 20 --no-pager

systemctl --user stop metricsd-test
systemctl --user reset-failed metricsd-test 2>/dev/null   # clears a failed run so the name can be reused
rm -rf ~/.local/state/metricsd-test
```

Pass `-config /path/to/file` after the binary to test export or other settings. Put that file outside `/tmp`, since `PrivateTmp=yes` hides the host's `/tmp` from the unit.
