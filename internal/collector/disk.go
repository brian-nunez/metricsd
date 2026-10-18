package collector

import (
	"os"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
)

// /proc/mounts escapes whitespace and backslashes in paths as octal.
var mountUnescaper = strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)

// Disk reports capacity and inode usage for each block-device-backed
// filesystem. Devices mounted more than once (e.g. btrfs subvolumes) are
// reported only for their first mount point.
type Disk struct {
	// Container also reports the container's root filesystem, which is an
	// overlay (or fuse-overlayfs when rootless) rather than a block device.
	Container bool
}

func (Disk) Name() string { return "disk" }

func (d Disk) Collect(time.Time) ([]Sample, error) {
	data, err := os.ReadFile("/proc/self/mounts")
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

		root := d.Container && mount == "/"
		if !root && (!strings.HasPrefix(dev, "/dev/") || seen[dev]) {
			continue
		}
		// Containers bind-mount single files such as /etc/hosts from the
		// host's disk; those describe the host, not the container.
		if fi, err := os.Stat(mount); err != nil || !fi.IsDir() {
			continue
		}
		if !root {
			seen[dev] = true
		}

		u, err := disk.Usage(mount)
		if err != nil {
			continue
		}
		l := labels("mount", mount)
		out = append(out,
			gauge("disk.total_bytes", l, float64(u.Total)),
			gauge("disk.used_bytes", l, float64(u.Used)),
			gauge("disk.free_bytes", l, float64(u.Free)),
			gauge("disk.used_pct", l, u.UsedPercent),
		)
		// btrfs and some network filesystems report no inode limits.
		if u.InodesTotal > 0 {
			out = append(out,
				gauge("disk.inodes_total", l, float64(u.InodesTotal)),
				gauge("disk.inodes_used", l, float64(u.InodesUsed)),
				gauge("disk.inodes_free", l, float64(u.InodesFree)),
				gauge("disk.inodes_used_pct", l, u.InodesUsedPercent),
			)
		}
	}
	return out, nil
}
