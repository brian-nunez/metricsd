package collector

import (
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// /proc/mounts escapes whitespace and backslashes in paths as octal.
var mountUnescaper = strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)

// Disk reports capacity for each block-device-backed filesystem. Devices
// mounted more than once (e.g. btrfs subvolumes) are reported only for
// their first mount point.
type Disk struct{}

func (Disk) Name() string { return "disk" }

func (Disk) Collect(time.Time) ([]Sample, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var out []Sample
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		dev, mount := f[0], mountUnescaper.Replace(f[1])
		if !strings.HasPrefix(dev, "/dev/") || seen[dev] {
			continue
		}
		seen[dev] = true

		var st unix.Statfs_t
		if err := unix.Statfs(mount, &st); err != nil {
			continue
		}

		bs := uint64(st.Frsize)
		if bs == 0 {
			bs = uint64(st.Bsize)
		}
		total := st.Blocks * bs
		used := (st.Blocks - st.Bfree) * bs
		avail := st.Bavail * bs

		labels := "mount=" + mount
		out = append(out,
			Sample{Metric: "disk.total_bytes", Labels: labels, Value: float64(total)},
			Sample{Metric: "disk.used_bytes", Labels: labels, Value: float64(used)},
			Sample{Metric: "disk.free_bytes", Labels: labels, Value: float64(avail)},
		)
		// Matches df: reserved blocks count as neither used nor available.
		if used+avail > 0 {
			out = append(out, Sample{Metric: "disk.used_pct", Labels: labels, Value: float64(used) / float64(used+avail) * 100})
		}
	}
	return out, nil
}
