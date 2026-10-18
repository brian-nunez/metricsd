# Installation

## Requirements

| Requirement            | Why                                                         |
| ---------------------- | ----------------------------------------------------------- |
| Linux                  | Metrics come from `/proc` and `/sys`                        |
| Go 1.27 or newer       | Build from source                                           |
| `make`, `install`, `sed` | Used by `make install`                                    |
| systemd (user session) | Runs the daemon as a service (optional, see below)          |

No root access is needed. Everything installs into your home directory and runs as your user.

## Install with make (recommended)

```bash
git clone git@github.com:brian-nunez/metricsd.git
cd metricsd
make install
systemctl --user enable --now metricsd
```

`make install` does the following:

| Step | Result |
| ---- | ------ |
| Builds both binaries | Static, CGO-free, stripped (`-trimpath -ldflags '-s -w'`) |
| Installs binaries | `~/.local/bin/metricsd`, `~/.local/bin/metricsctl` |
| Installs the unit | `~/.config/systemd/user/metricsd.service` with `ExecStart` pointing at the installed binary |
| Reloads systemd | `systemctl --user daemon-reload` |

### Custom install locations

All paths are overridable:

```bash
make install PREFIX=/opt/metricsd                 # binaries in /opt/metricsd/bin
make install BINDIR=$HOME/bin                      # binaries in ~/bin
make install UNITDIR=$HOME/.config/systemd/user    # unit location (default shown)
```

Pass the same variables to `make uninstall` later.

### Make sure the binaries are on your PATH

```bash
command -v metricsctl || echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
```

## Verify the install

```bash
systemctl --user status metricsd     # should be "active (running)"
metricsctl health                    # prints "ok"
metricsctl now                       # wait one interval (5s) for CPU and rate metrics
```

Rate-based metrics (CPU, disk IO, network) need two samples, so they appear after the second collection cycle.

## Keep it running after logout

User services stop when your last session ends. On a server or any machine where you want continuous collection, enable lingering once:

```bash
loginctl enable-linger "$USER"
```

## Install with go install

`go install` builds the binaries but does not install the systemd unit:

```bash
go install github.com/brian-nunez/metricsd/cmd/...@latest
```

Binaries land in `$(go env GOPATH)/bin`. To run it as a service, install the unit from a checkout pointing at that directory:

```bash
make install BINDIR="$(go env GOPATH)/bin"
```

Or create the unit by hand: copy `deploy/metricsd.service` to `~/.config/systemd/user/`, replace `@BINDIR@` with the binary's directory, and run `systemctl --user daemon-reload`.

## Run without systemd

`metricsd` runs fine in the foreground, which is useful for testing:

```bash
make build
./bin/metricsd                              # default config path
./bin/metricsd -config ./my-config.toml     # explicit config
```

Stop it with `Ctrl+C` (SIGINT) or SIGTERM; it shuts down cleanly. Logs go to stderr.

## Default file locations

| What     | Path                                         | Notes |
| -------- | -------------------------------------------- | ----- |
| Config   | `~/.config/metricsd/config.toml`             | Optional; defaults apply when missing |
| Database | `~/.local/state/metricsd/metrics.db`         | Plus `-wal` and `-shm` files |
| Socket   | `$XDG_RUNTIME_DIR/metricsd/metricsd.sock`    | Usually `/run/user/1000/...`; removed on stop |
| Unit     | `~/.config/systemd/user/metricsd.service`    | |
| Binaries | `~/.local/bin/metricsd`, `~/.local/bin/metricsctl` | |

XDG environment variables (`XDG_CONFIG_HOME`, `XDG_STATE_HOME`, `XDG_RUNTIME_DIR`) are honored.

## Next steps

- [Configure it](configuration.md) or start from an [example](../examples/README.md)
- [Learn metricsctl](usage.md)
- [Upgrade](#upgrading)

## Upgrading

```bash
cd metricsd
git pull
make install
systemctl --user restart metricsd
```

The database schema is created with `IF NOT EXISTS`, so existing data carries over.
