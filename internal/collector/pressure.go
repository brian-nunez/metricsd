package collector

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Pressure reports pressure stall information (PSI): the share of the last
// interval in which tasks were stalled waiting for CPU, memory, or IO.
// "some" means at least one task was stalled; "full" means all non-idle
// tasks were stalled at once.
type Pressure struct {
	// Files maps a resource name to its PSI file.
	Files map[string]string

	unavailable string
	counters    deltas
}

// HostPressure reads machine-wide PSI from /proc/pressure.
func HostPressure() *Pressure {
	return &Pressure{
		Files: map[string]string{
			"cpu":    "/proc/pressure/cpu",
			"memory": "/proc/pressure/memory",
			"io":     "/proc/pressure/io",
		},
		unavailable: "pressure stall information unavailable (kernel built without PSI or booted with psi=0)",
	}
}

// CgroupPressure reads PSI for a single cgroup.
func CgroupPressure(cg *Cgroup) *Pressure {
	return &Pressure{
		Files: map[string]string{
			"cpu":    filepath.Join(cg.Dir, "cpu.pressure"),
			"memory": filepath.Join(cg.Dir, "memory.pressure"),
			"io":     filepath.Join(cg.Dir, "io.pressure"),
		},
		unavailable: "no readable pressure files in " + cg.Dir,
	}
}

func (p *Pressure) Name() string { return "pressure" }

func (p *Pressure) Check() error {
	for _, file := range p.Files {
		if _, ok := readPSI(file); ok {
			return nil
		}
	}
	return errors.New(p.unavailable)
}

func (p *Pressure) Collect(now time.Time) ([]Sample, error) {
	cur := make(map[series]float64)
	for resource, file := range p.Files {
		totals, ok := readPSI(file)
		if !ok {
			continue
		}
		for kind, total := range totals {
			cur[series{"pressure." + kind + "_pct", labels("resource", resource)}] = total
		}
	}

	diff, secs, ok := p.counters.update(now, cur)
	if !ok {
		return nil, nil
	}
	out := make([]Sample, 0, len(diff))
	for s, stalledUsec := range diff {
		out = append(out, gauge(s.metric, s.labels, clampPct(stalledUsec/(secs*1e6)*100)))
	}
	return out, nil
}

// readPSI returns the cumulative stall time in microseconds for each line
// of a PSI file, e.g. "some avg10=0.00 avg60=0.00 avg300=0.00 total=1234".
func readPSI(path string) (map[string]float64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	out := make(map[string]float64, 2)
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || (f[0] != "some" && f[0] != "full") {
			continue
		}
		for _, kv := range f[1:] {
			if v, ok := strings.CutPrefix(kv, "total="); ok {
				if n, err := strconv.ParseFloat(v, 64); err == nil {
					out[f[0]] = n
				}
			}
		}
	}
	return out, len(out) > 0
}
