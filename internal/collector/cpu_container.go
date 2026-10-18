package collector

import (
	"errors"
	"runtime"
	"time"
)

// ContainerCPU reports CPU usage relative to the container's own CPU
// allowance: its quota (cpu.max) when set, otherwise the CPUs it may run
// on. A container limited to 1 CPU on a 64-core host reports 1 core and
// 100% when it uses all of it.
type ContainerCPU struct {
	Cgroup *Cgroup

	counters deltas
}

func (c *ContainerCPU) Name() string { return "cpu" }

func (c *ContainerCPU) Check() error {
	if !c.Cgroup.has("cpu.stat") {
		return errors.New("cpu.stat not found in " + c.Cgroup.Dir)
	}
	return nil
}

func (c *ContainerCPU) Collect(now time.Time) ([]Sample, error) {
	stat, ok := c.Cgroup.keyValues("cpu.stat")
	if !ok {
		return nil, errors.New("cpu.stat not readable")
	}

	// NumCPU honors the cpuset the container is pinned to.
	cores := float64(runtime.NumCPU())
	if limit, ok := c.Cgroup.cpuLimit(); ok && limit < cores {
		cores = limit
	}
	out := []Sample{gauge("cpu.cores", "", cores)}

	cur := make(map[series]float64)
	// nr_periods and throttling fields exist only when the cpu controller
	// is delegated; usage fields are always present.
	for _, k := range []string{"usage_usec", "user_usec", "system_usec", "nr_periods", "nr_throttled", "throttled_usec"} {
		if v, found := stat[k]; found {
			cur[series{k, ""}] = v
		}
	}

	diff, secs, ok := c.counters.update(now, cur)
	if !ok {
		return out, nil
	}
	capacityUsec := secs * 1e6 * cores
	if v, found := diff[series{"usage_usec", ""}]; found {
		out = append(out, gauge("cpu.usage_pct", labels("core", "all"), clampPct(pct(v, capacityUsec))))
	}
	for _, m := range []struct{ key, mode string }{{"user_usec", "user"}, {"system_usec", "system"}} {
		if v, found := diff[series{m.key, ""}]; found {
			out = append(out, gauge("cpu.mode_pct", labels("mode", m.mode), clampPct(pct(v, capacityUsec))))
		}
	}
	if periods := diff[series{"nr_periods", ""}]; periods > 0 {
		out = append(out, gauge("cpu.throttled_periods_pct", "", clampPct(pct(diff[series{"nr_throttled", ""}], periods))))
	}
	if v, found := diff[series{"throttled_usec", ""}]; found {
		out = append(out, gauge("cpu.throttled_time_pct", "", clampPct(v/(secs*1e6)*100)))
	}
	return out, nil
}
