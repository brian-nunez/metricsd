# minimal

A starting point that spells out the three settings people most often change. The values are the defaults, so this config behaves exactly like having no config file.

## Settings

| Key | Value | Meaning |
| --- | ----- | ------- |
| `interval` | `5s` | Sample every 5 seconds |
| `retention` | `1d` | Keep one day of history |
| `[net] exclude_interfaces` | `lo`, `veth*`, `br-*`, `docker*` | Skip loopback and Docker's virtual interfaces |

## When to use it

- You want a config file in place to edit later.
- You want to see which knobs matter without reading the full reference.

## Footprint

About 169 series on a typical desktop → **~98 MiB at full retention** (1 day). A container reports about 73 series (~42 MiB).

## Next steps

- Tune `interval` and `retention` using the [sizing table](../../docs/architecture.md#disk-usage).
- Find your interface names with `ip -br link` and adjust the exclude list.
- See [`deploy/config.example.toml`](../../deploy/config.example.toml) for every key.
