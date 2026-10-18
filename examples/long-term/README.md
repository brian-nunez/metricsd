# long-term

Keeps three months of history at one-minute resolution: enough to see trends, compare weeks, and plan disk or memory upgrades.

## Settings

| Key | Value | Why |
| --- | ----- | --- |
| `interval` | `60s` | Trend questions ("is disk usage growing?") don't need second-level detail. 12× fewer points than the default. |
| `retention` | `90d` | A full quarter: long enough to see monthly patterns and growth. |

## Footprint

About 169 series → **~8 MiB/day, ~730 MiB at full retention**. That's roughly the same size as the default 5s × 7d config, but covering 13× the time.

## Pair with lingering

Long-term history is only useful without gaps. On machines you log out of, enable lingering so metricsd keeps running:

```bash
loginctl enable-linger "$USER"
```

## Useful queries

```bash
# Root filesystem growth, one point per day
metricsctl history disk.used_pct -since 90d -step 1d -labels mount=/

# Weekly average memory use
metricsctl history mem.used_bytes -since 90d -step 7d

# Peak temperature per sensor over the quarter (straight SQL)
sqlite3 -readonly ~/.local/state/metricsd/metrics.db "
  SELECT s.labels, max(p.value) FROM points p JOIN series s ON s.id = p.series_id
  WHERE s.metric = 'temp.celsius' GROUP BY s.labels ORDER BY 2 DESC;"
```

Daily and weekly steps are averages, which smooth out peaks. Use SQL with `max()` when you need peaks.

## Variations

- **A full year:** `retention = "365d"` (~2.9 GiB). Consider `interval = "5m"` (~590 MiB) instead.
- **Fine detail for recent data:** metricsd stores one resolution. To keep both, run this config for history and query recent spikes from a separate high-resolution instance with its own `db_path` and `socket_path`.
