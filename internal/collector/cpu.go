package collector

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type cpuTimes struct {
	idle, total uint64
}

// CPU reports usage percentage for all cores combined and for each core.
// Usage is derived from the delta between two reads of /proc/stat.
type CPU struct {
	prev map[string]cpuTimes
}

func (c *CPU) Name() string { return "cpu" }

func (c *CPU) Collect(time.Time) ([]Sample, error) {
	cur, err := readCPUTimes()
	if err != nil {
		return nil, err
	}
	defer func() { c.prev = cur }()

	if c.prev == nil {
		return nil, nil
	}

	out := make([]Sample, 0, len(cur))
	for name, t := range cur {
		p, ok := c.prev[name]
		if !ok || t.total <= p.total || t.idle < p.idle {
			continue
		}
		busy := 1 - float64(t.idle-p.idle)/float64(t.total-p.total)

		core := "all"
		if name != "cpu" {
			core = strings.TrimPrefix(name, "cpu")
		}
		out = append(out, Sample{Metric: "cpu.usage_pct", Labels: "core=" + core, Value: busy * 100})
	}
	return out, nil
}

func readCPUTimes() (map[string]cpuTimes, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make(map[string]cpuTimes)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 5 || !strings.HasPrefix(fields[0], "cpu") {
			continue
		}

		var t cpuTimes
		// Columns: user nice system idle iowait irq softirq steal guest guest_nice.
		// guest and guest_nice are already counted in user and nice, so stop at steal.
		for i, v := range fields[1:] {
			if i >= 8 {
				break
			}
			n, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse /proc/stat %s: %w", fields[0], err)
			}
			t.total += n
			if i == 3 || i == 4 {
				t.idle += n
			}
		}
		out[fields[0]] = t
	}
	return out, sc.Err()
}
