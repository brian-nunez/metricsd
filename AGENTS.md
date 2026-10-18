# AGENTS.md

Linux metrics daemon (`metricsd`) and CLI (`metricsctl`) in Go. Layout is in `README.md`; build targets are in the `Makefile`.

## Verify every change

1. `gofmt -l .` prints nothing, `make vet` and `make build` pass.
2. Run the daemon end to end against a scratch config and exercise the changed path with `metricsctl`:
   ```toml
   interval    = "2s"
   db_path     = "/tmp/mdtest/metrics.db"
   socket_path = "/tmp/mdtest/metricsd.sock"
   ```
   `./bin/metricsd -config <file>` then `./bin/metricsctl -socket /tmp/mdtest/metricsd.sock now`. Keep socket paths at 107 bytes or fewer (the `sun_path` limit); deep temp directories exceed it.
3. Touching `deploy/metricsd.service` or anything that writes files or opens sockets: confirm it still runs under the sandbox (`systemd-run --user` with the unit's `[Service]` properties) — see `docs/systemd.md`.

Done means all three pass and the docs below match the new behaviour.

## Conventions the code relies on

- **Metric names**: `group.name_unit`. `metricsctl` picks display units from the suffix (`_bytes`, `_bytes_per_sec`, `_pct`, `.celsius`, `.seconds`); a new unit needs a matching case in `cmd/metricsctl/format.go`.
- **Labels**: one string of comma-separated `key=value` pairs, stable across reboots (prefer device addresses over `hwmonN`-style indexes). Labels plus metric identify a series in the `series` table.
- **Counter collectors** return nothing on their first call (baseline) and derive rates through `counterRate` in `internal/collector/collector.go`.
- **Collector registry**: a new collector is added in three places — `config.CollectorNames`, `buildCollectors` in `cmd/metricsd/main.go`, and `docs/metrics.md`.
- **`internal/wire` stays dependency-free** so `metricsctl` never links SQLite. The API converts store types to wire types.
- **Dependencies**: standard library first; the only third-party modules are `modernc.org/sqlite`, `golang.org/x/sys`, and `BurntSushi/toml`. Keep builds CGO-free.
- **Config**: every key has a default in `config.Default()`, is checked in `Validate()`, and is documented in `docs/configuration.md` and `deploy/config.example.toml`. Unknown keys are rejected, so renaming a key is a breaking change.
- **Sandbox**: the unit makes the filesystem read-only except `StateDirectory` and `RuntimeDirectory`, and allows only `AF_UNIX`. New writable paths or address families need a unit change.

## Docs move with code

User-facing behaviour is documented in `docs/` and `examples/`. Update the matching doc in the same change: flags → `docs/usage.md`, endpoints → `docs/api.md`, metrics → `docs/metrics.md`, config keys → `docs/configuration.md` plus any affected `examples/*/config.toml`.

## Commits

Conventional Commits (`feat:`, `fix:`, `docs:`, `refactor:`, `chore:`), authored by the repository owner with no tool attribution trailers. Code comments explain *why*, in the voice of the surrounding code.
