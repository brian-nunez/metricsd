package collector

import (
	"time"

	"github.com/shirou/gopsutil/v4/disk"
)

// HostDiskIO reports throughput, IOPS, latency, and utilization for whole
// block devices. Devices matching any Exclude glob are skipped.
type HostDiskIO struct {
	Exclude []string

	counters deltas
}

func (d *HostDiskIO) Name() string { return "diskio" }

func (d *HostDiskIO) Collect(now time.Time) ([]Sample, error) {
	stats, err := disk.IOCounters()
	if err != nil {
		return nil, err
	}

	cur := make(map[series]float64)
	var devices []string
	var out []Sample
	for name, s := range stats {
		// Partitions have no /sys/block entry; skipping them avoids double counting.
		if matchAny(d.Exclude, name) || !exists("/sys/block/"+name) {
			continue
		}
		l := labels("device", name)
		devices = append(devices, l)
		cur[series{"read_bytes", l}] = float64(s.ReadBytes)
		cur[series{"write_bytes", l}] = float64(s.WriteBytes)
		cur[series{"reads", l}] = float64(s.ReadCount)
		cur[series{"writes", l}] = float64(s.WriteCount)
		cur[series{"read_time", l}] = float64(s.ReadTime)
		cur[series{"write_time", l}] = float64(s.WriteTime)
		cur[series{"io_time", l}] = float64(s.IoTime)
		out = append(out, gauge("diskio.in_progress", l, float64(s.IopsInProgress)))
	}

	diff, secs, ok := d.counters.update(now, cur)
	if !ok {
		return out, nil
	}

	for _, l := range devices {
		get := func(m string) (float64, bool) {
			v, found := diff[series{m, l}]
			return v, found
		}
		for _, r := range []struct{ counter, metric string }{
			{"read_bytes", "diskio.read_bytes_per_sec"},
			{"write_bytes", "diskio.write_bytes_per_sec"},
			{"reads", "diskio.reads_per_sec"},
			{"writes", "diskio.writes_per_sec"},
		} {
			if v, found := get(r.counter); found {
				out = append(out, gauge(r.metric, l, v/secs))
			}
		}
		// Average time per completed request, as iostat's r_await/w_await.
		for _, r := range []struct{ ops, time, metric string }{
			{"reads", "read_time", "diskio.read_latency_ms"},
			{"writes", "write_time", "diskio.write_latency_ms"},
		} {
			ops, ok1 := get(r.ops)
			ms, ok2 := get(r.time)
			if ok1 && ok2 {
				latency := 0.0
				if ops > 0 {
					latency = ms / ops
				}
				out = append(out, gauge(r.metric, l, latency))
			}
		}
		if busy, found := get("io_time"); found {
			out = append(out, gauge("diskio.busy_pct", l, clampPct(busy/(secs*1000)*100)))
		}
	}
	return out, nil
}
