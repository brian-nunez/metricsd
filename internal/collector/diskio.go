package collector

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// /proc/diskstats always counts in 512-byte sectors, regardless of the
// device's physical sector size.
const sectorSize = 512

// DiskIO reports read/write throughput for whole block devices.
// Devices matching any Exclude glob are skipped.
type DiskIO struct {
	Exclude []string

	rate counterRate
}

func (d *DiskIO) Name() string { return "diskio" }

func (d *DiskIO) Collect(now time.Time) ([]Sample, error) {
	data, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		return nil, err
	}

	cur := make(map[string]uint64)
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) < 10 {
			continue
		}
		name := f[2]
		if matchAny(d.Exclude, name) {
			continue
		}
		// Partitions have no /sys/block entry; skipping them avoids double counting.
		if _, err := os.Stat("/sys/block/" + name); err != nil {
			continue
		}

		read, err1 := strconv.ParseUint(f[5], 10, 64)
		written, err2 := strconv.ParseUint(f[9], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		cur["read|"+name] = read * sectorSize
		cur["write|"+name] = written * sectorSize
	}

	rates := d.rate.update(now, cur)
	out := make([]Sample, 0, len(rates))
	for key, v := range rates {
		dir, dev, _ := strings.Cut(key, "|")
		out = append(out, Sample{Metric: "diskio." + dir + "_bytes_per_sec", Labels: "device=" + dev, Value: v})
	}
	return out, nil
}
