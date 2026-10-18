# Installation

## Requirements

| Requirement            | Why                                                         |
| ---------------------- | ----------------------------------------------------------- |
| Linux                  | Metrics come from `/proc` and `/sys`                        |
| Go 1.27 or newer       | Only for `go install` or building from source               |
| `make`, `install`, `sed` | Used by `make install` and the install snippets           |
| systemd (user session) | Runs the daemon as a service (optional, see below)          |

No root access is needed. Everything installs into your home directory and runs as your user.

## Install with go install (recommended)

Requires Go 1.27 or newer. Builds both binaries from the module, without a checkout:

```bash
go install github.com/brian-nunez/metricsd@latest
go install github.com/brian-nunez/metricsd/metricsctl@latest
```

The binaries go to `$(go env GOPATH)/bin` (or `$GOBIN` when set). `@latest` resolves to the newest release tag, and `metricsd -version` reports it.

Run it as a user service. `go install` installs only binaries, so the unit template comes from GitHub:

```bash
BIN=$(go env GOBIN); BIN=${BIN:-$(go env GOPATH)/bin}
mkdir -p ~/.config/systemd/user
curl -fsSL https://raw.githubusercontent.com/brian-nunez/metricsd/main/deploy/metricsd.service \
  | sed "s|@BINDIR@|$BIN|" > ~/.config/systemd/user/metricsd.service
systemctl --user daemon-reload
systemctl --user enable --now metricsd
```

**Upgrade:** run both `go install` commands again, then `systemctl --user restart metricsd`.

## Install a prebuilt release with curl

No Go needed. Every release on [GitHub](https://github.com/brian-nunez/metricsd/releases) has static Linux binaries for `amd64`, `arm64`, and `armv7`:

```bash
ARCH=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/; s/armv7l/armv7/')
curl -fsSL "https://github.com/brian-nunez/metricsd/releases/latest/download/metricsd_linux_$ARCH.tar.gz" | tar -xz
install -Dm755 -t ~/.local/bin "metricsd_linux_$ARCH/metricsd" "metricsd_linux_$ARCH/metricsctl"
```

Each archive holds `metricsd`, `metricsctl`, the systemd unit template, the reference config, `LICENSE`, and `README.md`. To install a specific version, replace `latest/download` with `download/v0.1.0`.

To check the download against the published checksums, download the archive to a file first:

```bash
BASE=https://github.com/brian-nunez/metricsd/releases/latest/download
curl -fsSLO "$BASE/metricsd_linux_$ARCH.tar.gz"
curl -fsSLO "$BASE/checksums.txt"
sha256sum --ignore-missing -c checksums.txt     # must print OK
tar -xzf "metricsd_linux_$ARCH.tar.gz"
```

Run it as a user service with the unit template from the archive:

```bash
mkdir -p ~/.config/systemd/user
sed "s|@BINDIR@|$HOME/.local/bin|" "metricsd_linux_$ARCH/metricsd.service" > ~/.config/systemd/user/metricsd.service
systemctl --user daemon-reload
systemctl --user enable --now metricsd
```

**Upgrade:** run the curl and `install` commands again, then `systemctl --user restart metricsd`.

## Install from source with make

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
metricsd -version                    # prints the build version
systemctl --user status metricsd     # should be "active (running)"
metricsctl health                    # prints "ok"
metricsctl info                      # environment detected (physical, vm, ...) and its ID
metricsctl now                       # wait one interval (5s) for CPU and rate metrics
```

Rate-based metrics (CPU, disk IO, network) need two samples, so they appear after the second collection cycle.

## Keep it running after logout

User services stop when your last session ends. On a server or any machine where you want continuous collection, enable lingering once:

```bash
loginctl enable-linger "$USER"
```

## Docker image

To measure a container, metricsd runs inside it (see [containers.md](containers.md)). Build the image from the repository:

```bash
docker build -t metricsd .      # or: podman build -t metricsd .
docker build --build-arg VERSION=v0.1.0 -t metricsd:v0.1.0 .   # stamp a version; otherwise metricsd -version prints "dev"
```

| Property | Value |
| -------- | ----- |
| Base | `scratch`: only the two static binaries, CA certificates for OTLP over HTTPS, and a world-writable `/tmp` |
| Size | about 38 MB |
| Binaries | `/usr/local/bin/metricsd`, `/usr/local/bin/metricsctl` |
| Entrypoint | `metricsd` |

Copy the binaries into your application image:

```dockerfile
COPY --from=metricsd:latest /usr/local/bin/ /usr/local/bin/
```

Then start `metricsd` alongside your application; [containers.md](containers.md#adding-metricsd-to-an-image) shows entrypoints for that.

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
| Database | `~/.local/state/metricsd/metrics.db`         | Plus `-wal` and `-shm` files. In containers: `/var/lib/metricsd` as root, `/tmp/metricsd` otherwise |
| Socket   | `$XDG_RUNTIME_DIR/metricsd/metricsd.sock`    | Usually `/run/user/1000/...`; removed on stop. Falls back to `/tmp/metricsd/metricsd.sock` when there is no runtime directory (containers) |
| Unit     | `~/.config/systemd/user/metricsd.service`    | |
| Binaries | `~/.local/bin/metricsd`, `~/.local/bin/metricsctl` | |

XDG environment variables (`XDG_CONFIG_HOME`, `XDG_STATE_HOME`, `XDG_RUNTIME_DIR`) are honored.

## Next steps

- [Configure it](configuration.md) or start from an [example](../examples/README.md)
- [Send metrics to a central server](export.md)
- [Run it in containers](containers.md)
- [Learn metricsctl](usage.md)
- [Upgrade](#upgrading)

## Upgrading

```bash
cd metricsd
git pull
make install
```

`make install` restarts the service when it is already running, so the new binary takes over immediately. On a first install it only prints how to start it.

The database schema is created with `IF NOT EXISTS`, so existing data carries over.
