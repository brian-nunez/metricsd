# Uninstall

## With make

From the repository checkout:

```bash
make uninstall
```

This stops and disables the service, removes both binaries and the unit file, and reloads systemd. If you installed with custom `PREFIX`, `BINDIR`, or `UNITDIR`, pass the same values:

```bash
make uninstall BINDIR=$HOME/bin
```

Your data and config are left in place on purpose.

## Remove data and config

```bash
rm -rf "${XDG_STATE_HOME:-$HOME/.local/state}/metricsd"    # database
rm -rf "${XDG_CONFIG_HOME:-$HOME/.config}/metricsd"        # config
```

If you set a custom `db_path`, delete that directory instead. If you added a systemd drop-in with `systemctl --user edit metricsd`, remove it too:

```bash
rm -rf ~/.config/systemd/user/metricsd.service.d
```

## Without the repository

If the checkout is gone, do the same steps by hand:

```bash
systemctl --user disable --now metricsd
rm -f ~/.local/bin/metricsd ~/.local/bin/metricsctl
rm -f ~/.config/systemd/user/metricsd.service
rm -rf ~/.config/systemd/user/metricsd.service.d
systemctl --user daemon-reload
systemctl --user reset-failed metricsd 2>/dev/null

# Optional: data and config
rm -rf ~/.local/state/metricsd ~/.config/metricsd
```

If you installed with `go install`, the binaries are in `$(go env GOPATH)/bin` instead.

## Disable lingering

If you enabled lingering only for metricsd:

```bash
loginctl disable-linger "$USER"
```

## Verify

```bash
systemctl --user status metricsd    # "Unit metricsd.service could not be found."
command -v metricsd metricsctl      # no output
ls "$XDG_RUNTIME_DIR/metricsd"      # "No such file or directory"
```
