# AGENTS.md

Linux metrics agent (`metricsd`) and CLI (`metricsctl`) in Go. It measures the environment it runs in (machine or container) and exports over OTLP. Layout is in `README.md`; build targets are in the `Makefile`.

## Verify every change

1. `gofmt -l .` prints nothing, `make vet` and `make build` pass.
2. Run the daemon end to end against a scratch config and exercise the changed path with `metricsctl`:
   ```toml
   interval    = "2s"
   db_path     = "/tmp/mdtest/metrics.db"
   socket_path = "/tmp/mdtest/metricsd.sock"
   ```
   `./bin/metricsd -config <file>` then `./bin/metricsctl -socket /tmp/mdtest/metricsd.sock now`. Keep socket paths at 107 bytes or fewer (the `sun_path` limit).
3. Container paths: bind-mount `bin/` into a limited container and compare with `docker stats`:
   `docker run --rm --memory=1g --cpus=1 -v $PWD/bin:/opt/md:ro alpine /opt/md/metricsd`, then `docker exec ... /opt/md/metricsctl now`. Also run once with `--user 1000:1000` (rootless-style non-root paths).
4. Hardware collectors (temp, fan, battery) on a machine without that hardware: bind-mount a simulated sysfs tree over `/sys/class/hwmon` or `/sys/class/power_supply` in a container and set `environment = "physical"` in its config.
5. Export changes: run a local OpenTelemetry Collector with the `debug` exporter on non-default host ports (4317/4318 are often taken) and check its log — `docs/export.md` has the config.
6. Touching `deploy/metricsd.service` or anything that writes files or opens sockets: run under the sandbox with `systemd-run --user` as in `docs/systemd.md` (array-quoted properties, separate test state/runtime dirs).

Done means every applicable step passes and the docs below match the new behaviour.

## Conventions the code relies on

- **Metric names**: `group.name_unit`. `wire.Unit` maps the suffix to a UCUM unit used by both `metricsctl` formatting and OTLP export; a new suffix needs a case there and in `cmd/metricsctl/format.go`.
- **Labels**: built with `labels()` (sanitizes `,` and `=`), stable across reboots (device addresses, not `hwmonN`). Metric plus labels identify a series.
- **Counters** go through `deltas` in `internal/collector/collector.go`; rate collectors return nothing on their first call.
- **Collector registry**: a collector is registered in `config.CollectorNames`, in `buildCollectors` (`cmd/metricsd/main.go`) in the host branch, the container branch, or both, and documented in `docs/metrics.md`.
- **Environments**: containers read cgroup v2 files through `collector.Cgroup`, where every file is optional (rootless runtimes delegate only some controllers); missing data is omitted, never zero. Machine-wide figures (load, sensors) are host-only.
- **`Checker`** is for sources absent by environment (cgroup controllers, PSI) and disables a collector once at startup. Hardware collectors stay always-on so hot-plugged batteries and late-loading sensor drivers are picked up.
- **Exporters** implement `export.Exporter` and receive `store.Cycle`s in metricsd's own format; the store's export cursor is the delivery buffer.
- **`internal/wire` stays standard-library only** so `metricsctl` never links SQLite, gopsutil, or OpenTelemetry (`go list -deps ./cmd/metricsctl`).
- **Dependencies**: gopsutil v4 (host metrics), OpenTelemetry SDK + OTLP HTTP/gRPC exporters (with grpc), `modernc.org/sqlite`, `BurntSushi/toml`, `golang.org/x/sys`. cgroup files are read directly because library readers zero-fill missing controllers and log on every read, which breaks rootless correctness. Keep builds CGO-free.
- **Config**: every key has a default in `config.Default()`, is checked in `Validate()`, and is documented in `docs/configuration.md` and `deploy/config.example.toml`. Unknown keys are rejected, so renaming a key is a breaking change.
- **Sandbox**: the unit makes the filesystem read-only except `StateDirectory` and `RuntimeDirectory`, and allows `AF_UNIX`, `AF_INET`, `AF_INET6`. New writable paths or address families need a unit change.

## Docs move with code

User-facing behaviour is documented in `docs/` and `examples/`. Update the matching doc in the same change: flags → `docs/usage.md`, endpoints → `docs/api.md`, metrics → `docs/metrics.md`, config keys → `docs/configuration.md` plus affected `examples/*/config.toml`, export → `docs/export.md`, container behaviour → `docs/containers.md`.

## Commits

Conventional Commits (`feat:`, `fix:`, `docs:`, `refactor:`, `chore:`), authored by the repository owner with no tool attribution trailers. Code comments explain *why*, in the voice of the surrounding code.
