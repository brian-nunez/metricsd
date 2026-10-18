package collector

import (
	"errors"
	"time"

	"github.com/shirou/gopsutil/v4/mem"
)

// ContainerMem reports memory and swap usage against the container's own
// limits. Without a limit, the host's capacity is the ceiling, since
// that is what the container can actually use.
type ContainerMem struct {
	Cgroup *Cgroup

	counters deltas
}

func (m *ContainerMem) Name() string { return "mem" }

func (m *ContainerMem) Check() error {
	if !m.Cgroup.has("memory.current") {
		return errors.New("memory controller not delegated to " + m.Cgroup.Dir)
	}
	return nil
}

func (m *ContainerMem) Collect(now time.Time) ([]Sample, error) {
	current, _, ok := m.Cgroup.value("memory.current")
	if !ok {
		return nil, errors.New("memory.current not readable")
	}
	stat, _ := m.Cgroup.keyValues("memory.stat")

	// /proc/meminfo inside a container describes the host.
	host, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}

	total := float64(host.Total)
	if limit, unlimited, ok := m.Cgroup.value("memory.max"); ok && !unlimited && limit < total {
		total = limit
	}
	// Same as `docker stats`: reclaimable page cache doesn't count as used.
	used := max(0, current-stat["inactive_file"])

	out := []Sample{
		gauge("mem.total_bytes", "", total),
		gauge("mem.used_bytes", "", used),
		gauge("mem.available_bytes", "", max(0, total-used)),
		gauge("mem.used_pct", "", pct(used, total)),
	}
	if stat != nil {
		kernel, found := stat["kernel"]
		if !found {
			kernel = stat["kernel_stack"] + stat["slab"]
		}
		out = append(out,
			gauge("mem.cached_bytes", "", stat["file"]),
			gauge("mem.anon_bytes", "", stat["anon"]),
			gauge("mem.kernel_bytes", "", kernel),
			gauge("mem.slab_bytes", "", stat["slab"]),
			gauge("mem.shared_bytes", "", stat["shmem"]),
			gauge("mem.mapped_bytes", "", stat["file_mapped"]),
			gauge("mem.dirty_bytes", "", stat["file_dirty"]),
			gauge("mem.writeback_bytes", "", stat["file_writeback"]),
			gauge("mem.socket_bytes", "", stat["sock"]),
		)
	}
	if peak, _, ok := m.Cgroup.value("memory.peak"); ok {
		out = append(out, gauge("mem.peak_bytes", "", peak))
	}
	if events, ok := m.Cgroup.keyValues("memory.events"); ok {
		out = append(out, gauge("mem.oom_kills", "", events["oom_kill"]))
	}

	// Swap accounting is absent when the host disables it.
	if swapUsed, _, ok := m.Cgroup.value("memory.swap.current"); ok {
		swapTotal := float64(host.SwapTotal)
		if limit, unlimited, ok := m.Cgroup.value("memory.swap.max"); ok && !unlimited && limit < swapTotal {
			swapTotal = limit
		}
		out = append(out,
			gauge("swap.total_bytes", "", swapTotal),
			gauge("swap.used_bytes", "", swapUsed),
			gauge("swap.free_bytes", "", max(0, swapTotal-swapUsed)),
			gauge("swap.used_pct", "", pct(swapUsed, swapTotal)),
		)
	}

	if stat != nil {
		out = append(out, m.counters.rates(now, map[series]float64{
			{"mem.page_faults_per_sec", ""}:       stat["pgfault"],
			{"mem.major_page_faults_per_sec", ""}: stat["pgmajfault"],
		})...)
	}
	return out, nil
}
