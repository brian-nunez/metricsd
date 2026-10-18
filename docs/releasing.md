# Releasing

Releases publish prebuilt Linux binaries on [GitHub Releases](https://github.com/brian-nunez/metricsd/releases). The [release workflow](../.github/workflows/release.yml) runs when a tag that starts with `v` is pushed.

## Cut a release

```bash
git switch main && git pull
git tag v0.1.0
git push origin v0.1.0
```

The workflow then:

1. Runs `make vet`.
2. Runs `make dist VERSION=v0.1.0`, which builds `amd64`, `arm64`, and `armv7` archives and `checksums.txt`.
3. Creates the GitHub release with generated notes and attaches the archives.

A tag with a hyphen, such as `v0.2.0-rc.1`, is published as a pre-release, so `releases/latest` keeps pointing at the last stable version.

## Assets

| File | Contents |
| ---- | -------- |
| `metricsd_linux_amd64.tar.gz` | x86-64 |
| `metricsd_linux_arm64.tar.gz` | 64-bit ARM (Graviton, Ampere, Raspberry Pi 4/5 with a 64-bit OS) |
| `metricsd_linux_armv7.tar.gz` | 32-bit ARM (Raspberry Pi with a 32-bit OS) |
| `checksums.txt` | SHA-256 of each archive |

Each archive holds `metricsd`, `metricsctl`, `metricsd.service` (unit template with `@BINDIR@`), `config.example.toml`, `LICENSE`, and `README.md`. Asset names carry no version, so `https://github.com/brian-nunez/metricsd/releases/latest/download/<asset>` always serves the newest stable release.

## Versions

| Install method | `metricsd -version` |
| -------------- | ------------------- |
| Release archive | The tag, set at link time by `make dist VERSION=...` |
| `go install ...@v0.1.0` or `@latest` | The module version Go resolved |
| Build from a git checkout | A VCS pseudo-version, e.g. `v0.0.0-20261018061046-3d0cacb77f40` |

## Build the archives locally

```bash
make dist VERSION=v0.0.0-test
ls dist/
```
