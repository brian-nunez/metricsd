package collector

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
)

var cpuModes = []string{"user", "nice", "system", "idle", "iowait", "irq", "softirq", "steal"}

// HostCPU reports usage per core, a breakdown by CPU mode, clock
// frequency, and scheduler activity for a whole machine.
type HostCPU struct {
	times deltas
	stat  deltas
}

func (c *HostCPU) Name() string { return "cpu" }

func (c *HostCPU) Collect(now time.Time) ([]Sample, error) {
	total, err := cpu.Times(false)
	if err != nil {
		return nil, err
	}
	perCore, err := cpu.Times(true)
	if err != nil {
		return nil, err
	}

	cur := make(map[series]float64)
	record := func(t cpu.TimesStat, core string) {
		// guest and guest_nice are already counted in user and nice.
		modes := []float64{t.User, t.Nice, t.System, t.Idle, t.Iowait, t.Irq, t.Softirq, t.Steal}
		var sum float64
		for i, v := range modes {
			sum += v
			if core == "all" {
				cur[series{"mode", cpuModes[i]}] = v
			}
		}
		cur[series{"total", core}] = sum
		cur[series{"idle", core}] = t.Idle + t.Iowait
	}
	if len(total) > 0 {
		record(total[0], "all")
	}
	for _, t := range perCore {
		record(t, strings.TrimPrefix(t.CPU, "cpu"))
	}

	var out []Sample
	if n, err := cpu.Counts(true); err == nil && n > 0 {
		out = append(out, gauge("cpu.cores", "", float64(n)))
	}
	if n, err := cpu.Counts(false); err == nil && n > 0 {
		out = append(out, gauge("cpu.physical_cores", "", float64(n)))
	}

	if diff, _, ok := c.times.update(now, cur); ok {
		for s, sum := range diff {
			if s.metric != "total" || sum <= 0 {
				continue
			}
			// iowait can tick backwards on some kernels; skip the core for
			// one cycle rather than report it as fully busy.
			idle, found := diff[series{"idle", s.labels}]
			if !found {
				continue
			}
			out = append(out, gauge("cpu.usage_pct", labels("core", s.labels), clampPct(100-pct(idle, sum))))
		}
		if all := diff[series{"total", "all"}]; all > 0 {
			for _, m := range cpuModes {
				if v, found := diff[series{"mode", m}]; found {
					out = append(out, gauge("cpu.mode_pct", labels("mode", m), pct(v, all)))
				}
			}
		}
	}

	if counters, err := readProcStatCounters(); err == nil {
		out = append(out, c.stat.rates(now, counters)...)
	}
	return append(out, cpuFrequencies()...), nil
}

var procStatRates = map[string]string{
	"ctxt":    "cpu.context_switches_per_sec",
	"intr":    "cpu.interrupts_per_sec",
	"softirq": "cpu.softirqs_per_sec",
}

func readProcStatCounters() (map[series]float64, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil, err
	}
	cur := make(map[series]float64, len(procStatRates))
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		metric, ok := procStatRates[f[0]]
		if !ok {
			continue
		}
		// intr and softirq lines start with the total, followed by per-source counts.
		if v, err := strconv.ParseFloat(f[1], 64); err == nil {
			cur[series{metric, ""}] = v
		}
	}
	return cur, nil
}

// cpuFrequencies reads each core's current clock. VMs and some ARM boards
// expose no cpufreq data, in which case nothing is reported.
func cpuFrequencies() []Sample {
	paths, _ := filepath.Glob("/sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_cur_freq")
	out := make([]Sample, 0, len(paths))
	for _, p := range paths {
		khz, ok := readFloat(p)
		if !ok {
			continue
		}
		core := strings.TrimPrefix(filepath.Base(filepath.Dir(filepath.Dir(p))), "cpu")
		out = append(out, gauge("cpu.frequency_mhz", labels("core", core), khz/1000))
	}
	return out
}

func clampPct(v float64) float64 {
	return max(0, min(100, v))
}
