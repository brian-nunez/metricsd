package collector

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const cgroupRoot = "/sys/fs/cgroup"

// Cgroup reads cgroup v2 interface files for the cgroup metricsd runs in.
// Every file is optional: rootless Docker and Podman often get only some
// controllers delegated (commonly memory and pids, not cpu or io), and a
// missing file means "unknown", never zero.
type Cgroup struct {
	Dir string
}

// FindCgroup locates the cgroup of the current process. With a private
// cgroup namespace (the default for Docker and Podman on cgroup v2,
// rootful or rootless) /proc/self/cgroup reads "0::/" and the cgroup is
// the mount root itself.
func FindCgroup() (*Cgroup, error) {
	if !exists(filepath.Join(cgroupRoot, "cgroup.controllers")) {
		return nil, errors.New("cgroup v2 is not mounted at /sys/fs/cgroup (cgroup v1 hosts are not supported for container metrics)")
	}

	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		rel, ok := strings.CutPrefix(line, "0::")
		if !ok {
			continue
		}
		dir := filepath.Join(cgroupRoot, rel)
		if exists(filepath.Join(dir, "cgroup.controllers")) {
			return &Cgroup{Dir: dir}, nil
		}
	}
	return &Cgroup{Dir: cgroupRoot}, nil
}

func (c *Cgroup) has(file string) bool {
	return exists(filepath.Join(c.Dir, file))
}

// value reads a single-value file such as memory.max. unlimited is true
// when the file holds "max".
func (c *Cgroup) value(file string) (v float64, unlimited, ok bool) {
	s := readTrimmed(filepath.Join(c.Dir, file))
	switch s {
	case "":
		return 0, false, false
	case "max":
		return 0, true, true
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, false, err == nil
}

// keyValues reads flat keyed files such as cpu.stat and memory.stat.
func (c *Cgroup) keyValues(file string) (map[string]float64, bool) {
	data, err := os.ReadFile(filepath.Join(c.Dir, file))
	if err != nil {
		return nil, false
	}
	out := make(map[string]float64)
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		if v, err := strconv.ParseFloat(f[1], 64); err == nil {
			out[f[0]] = v
		}
	}
	return out, true
}

// cpuLimit returns the CPU quota in cores from cpu.max ("quota period").
// ok is false when there is no quota or the cpu controller is absent.
func (c *Cgroup) cpuLimit() (cores float64, ok bool) {
	f := strings.Fields(readTrimmed(filepath.Join(c.Dir, "cpu.max")))
	if len(f) != 2 || f[0] == "max" {
		return 0, false
	}
	quota, err1 := strconv.ParseFloat(f[0], 64)
	period, err2 := strconv.ParseFloat(f[1], 64)
	if err1 != nil || err2 != nil || period <= 0 {
		return 0, false
	}
	return quota / period, true
}

// ioStats parses io.stat lines such as
// "259:0 rbytes=1024 wbytes=2048 rios=1 wios=2 dbytes=0 dios=0",
// keyed by "major:minor".
func (c *Cgroup) ioStats() (map[string]map[string]float64, bool) {
	data, err := os.ReadFile(filepath.Join(c.Dir, "io.stat"))
	if err != nil {
		return nil, false
	}
	out := make(map[string]map[string]float64)
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		fields := make(map[string]float64, len(f)-1)
		for _, kv := range f[1:] {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				continue
			}
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				fields[k] = n
			}
		}
		out[f[0]] = fields
	}
	return out, true
}
