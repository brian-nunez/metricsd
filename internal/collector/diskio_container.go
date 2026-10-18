package collector

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ContainerDiskIO reports the container's own block IO from io.stat, per
// device. Devices matching any Exclude glob are skipped.
type ContainerDiskIO struct {
	Cgroup  *Cgroup
	Exclude []string

	counters deltas
}

func (d *ContainerDiskIO) Name() string { return "diskio" }

func (d *ContainerDiskIO) Check() error {
	if !d.Cgroup.has("io.stat") {
		return errors.New("io controller not delegated to " + d.Cgroup.Dir)
	}
	return nil
}

func (d *ContainerDiskIO) Collect(now time.Time) ([]Sample, error) {
	stats, ok := d.Cgroup.ioStats()
	if !ok {
		return nil, errors.New("io.stat not readable")
	}

	cur := make(map[series]float64)
	for majMin, f := range stats {
		if exists(filepath.Join("/sys/dev/block", majMin, "partition")) {
			continue
		}
		name := blockDeviceName(majMin)
		if matchAny(d.Exclude, name) {
			continue
		}
		l := labels("device", name)
		cur[series{"diskio.read_bytes_per_sec", l}] = f["rbytes"]
		cur[series{"diskio.write_bytes_per_sec", l}] = f["wbytes"]
		cur[series{"diskio.reads_per_sec", l}] = f["rios"]
		cur[series{"diskio.writes_per_sec", l}] = f["wios"]
	}
	return d.counters.rates(now, cur), nil
}

// blockDeviceName maps "major:minor" to a kernel device name such as
// nvme0n1, falling back to the numbers when /sys is not visible.
func blockDeviceName(majMin string) string {
	data, err := os.ReadFile(filepath.Join("/sys/dev/block", majMin, "uevent"))
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if name, ok := strings.CutPrefix(line, "DEVNAME="); ok {
				return name
			}
		}
	}
	return majMin
}
