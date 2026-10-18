package collector

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Mem reports RAM and swap usage from /proc/meminfo.
type Mem struct{}

func (Mem) Name() string { return "mem" }

func (Mem) Collect(time.Time) ([]Sample, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return nil, err
	}

	kb := make(map[string]float64)
	for _, line := range strings.Split(string(data), "\n") {
		key, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		f := strings.Fields(rest)
		if len(f) == 0 {
			continue
		}
		n, err := strconv.ParseFloat(f[0], 64)
		if err != nil {
			continue
		}
		kb[key] = n
	}

	for _, k := range []string{"MemTotal", "MemAvailable", "SwapTotal", "SwapFree"} {
		if _, ok := kb[k]; !ok {
			return nil, fmt.Errorf("/proc/meminfo: missing %s", k)
		}
	}

	const kib = 1024
	return []Sample{
		{Metric: "mem.total_bytes", Value: kb["MemTotal"] * kib},
		{Metric: "mem.available_bytes", Value: kb["MemAvailable"] * kib},
		{Metric: "mem.used_bytes", Value: (kb["MemTotal"] - kb["MemAvailable"]) * kib},
		{Metric: "swap.total_bytes", Value: kb["SwapTotal"] * kib},
		{Metric: "swap.used_bytes", Value: (kb["SwapTotal"] - kb["SwapFree"]) * kib},
	}, nil
}
