package collector

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/process"
)

// HostSystem reports uptime, process and thread counts, open file
// handles, and logged-in users for a whole machine.
type HostSystem struct {
	counters deltas
}

func (s *HostSystem) Name() string { return "system" }

func (s *HostSystem) Collect(now time.Time) ([]Sample, error) {
	var out []Sample
	if up, err := host.Uptime(); err == nil {
		out = append(out, gauge("uptime.seconds", "", float64(up)))
	}
	if pids, err := process.Pids(); err == nil {
		out = append(out, gauge("procs.count", "", float64(len(pids))))
	}
	if misc, err := load.Misc(); err == nil {
		out = append(out,
			gauge("procs.running", "", float64(misc.ProcsRunning)),
			gauge("procs.blocked", "", float64(misc.ProcsBlocked)),
		)
		out = append(out, s.counters.rates(now, map[series]float64{
			{"procs.created_per_sec", ""}: float64(misc.ProcsCreated),
		})...)
	}
	if threads, ok := threadCount(); ok {
		out = append(out, gauge("procs.threads", "", threads))
	}
	if open, limit, ok := openFiles(); ok {
		out = append(out, gauge("fs.open_files", "", open))
		// Recent kernels report LONG_MAX, meaning no limit.
		if limit < 1<<62 {
			out = append(out, gauge("fs.open_files_max", "", limit))
		}
	}
	// utmp is missing on some minimal and newer systemd distributions.
	if users, err := host.Users(); err == nil {
		out = append(out, gauge("users.logged_in", "", float64(len(users))))
	}

	if len(out) == 0 {
		return nil, errors.New("no system metrics readable")
	}
	return out, nil
}

// threadCount reads the number of kernel scheduling entities (threads)
// from the fourth field of /proc/loadavg, e.g. "1/1234".
func threadCount() (float64, bool) {
	f := strings.Fields(readTrimmed("/proc/loadavg"))
	if len(f) < 4 {
		return 0, false
	}
	_, total, ok := strings.Cut(f[3], "/")
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseFloat(total, 64)
	return v, err == nil
}

// openFiles reads allocated and maximum file handles from
// /proc/sys/fs/file-nr ("allocated unused max").
func openFiles() (open, limit float64, ok bool) {
	f := strings.Fields(readTrimmed("/proc/sys/fs/file-nr"))
	if len(f) < 3 {
		return 0, 0, false
	}
	allocated, err1 := strconv.ParseFloat(f[0], 64)
	unused, err2 := strconv.ParseFloat(f[1], 64)
	maxFiles, err3 := strconv.ParseFloat(f[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, false
	}
	return allocated - unused, maxFiles, true
}

// ContainerSystem reports uptime, processes, and the task limit for the
// container metricsd runs in. Process counts come from /proc, which a
// container's PID namespace already scopes to the container.
type ContainerSystem struct {
	Cgroup *Cgroup
}

func (s *ContainerSystem) Name() string { return "system" }

func (s *ContainerSystem) Collect(now time.Time) ([]Sample, error) {
	var out []Sample
	// PID 1 is the container's init process, so its age is the container's uptime.
	if p, err := process.NewProcess(1); err == nil {
		if created, err := p.CreateTime(); err == nil {
			out = append(out, gauge("uptime.seconds", "", max(0, now.Sub(time.UnixMilli(created)).Seconds())))
		}
	}
	if pids, err := process.Pids(); err == nil {
		out = append(out, gauge("procs.count", "", float64(len(pids))))
	}
	if s.Cgroup != nil {
		// pids.current counts every task (thread) in the cgroup.
		if v, _, ok := s.Cgroup.value("pids.current"); ok {
			out = append(out, gauge("procs.threads", "", v))
		}
		if v, unlimited, ok := s.Cgroup.value("pids.max"); ok && !unlimited {
			out = append(out, gauge("procs.limit", "", v))
		}
	}

	if len(out) == 0 {
		return nil, errors.New("no system metrics readable")
	}
	return out, nil
}
