package collector

import (
	"time"

	"github.com/shirou/gopsutil/v4/mem"
)

// HostMem reports RAM, swap, and paging activity for a whole machine.
type HostMem struct {
	counters deltas
}

func (m *HostMem) Name() string { return "mem" }

func (m *HostMem) Collect(now time.Time) ([]Sample, error) {
	vm, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}
	sw, err := mem.SwapMemory()
	if err != nil {
		return nil, err
	}

	total := float64(vm.Total)
	// Used matches `free`: memory the kernel cannot reclaim. Page cache
	// counts as available, not used.
	used := max(0, total-float64(vm.Available))

	out := []Sample{
		gauge("mem.total_bytes", "", total),
		gauge("mem.available_bytes", "", float64(vm.Available)),
		gauge("mem.used_bytes", "", used),
		gauge("mem.used_pct", "", pct(used, total)),
		gauge("mem.free_bytes", "", float64(vm.Free)),
		gauge("mem.buffers_bytes", "", float64(vm.Buffers)),
		gauge("mem.cached_bytes", "", float64(vm.Cached)),
		gauge("mem.shared_bytes", "", float64(vm.Shared)),
		gauge("mem.slab_bytes", "", float64(vm.Slab)),
		gauge("mem.active_bytes", "", float64(vm.Active)),
		gauge("mem.inactive_bytes", "", float64(vm.Inactive)),
		gauge("mem.dirty_bytes", "", float64(vm.Dirty)),
		gauge("mem.writeback_bytes", "", float64(vm.WriteBack)),
		gauge("mem.mapped_bytes", "", float64(vm.Mapped)),
		gauge("mem.page_tables_bytes", "", float64(vm.PageTables)),
		gauge("mem.committed_bytes", "", float64(vm.CommittedAS)),
		gauge("mem.commit_limit_bytes", "", float64(vm.CommitLimit)),
	}
	if vm.HugePagesTotal > 0 {
		out = append(out,
			gauge("mem.hugepages_total", "", float64(vm.HugePagesTotal)),
			gauge("mem.hugepages_free", "", float64(vm.HugePagesFree)),
		)
	}

	swapTotal := float64(sw.Total)
	swapUsed := max(0, swapTotal-float64(sw.Free))
	out = append(out,
		gauge("swap.total_bytes", "", swapTotal),
		gauge("swap.used_bytes", "", swapUsed),
		gauge("swap.free_bytes", "", float64(sw.Free)),
		gauge("swap.used_pct", "", pct(swapUsed, swapTotal)),
		gauge("swap.cached_bytes", "", float64(vm.SwapCached)),
	)

	return append(out, m.counters.rates(now, map[series]float64{
		{"swap.in_bytes_per_sec", ""}:         float64(sw.Sin),
		{"swap.out_bytes_per_sec", ""}:        float64(sw.Sout),
		{"mem.page_faults_per_sec", ""}:       float64(sw.PgFault),
		{"mem.major_page_faults_per_sec", ""}: float64(sw.PgMajFault),
	})...), nil
}
