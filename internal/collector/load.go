package collector

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Load reports the 1/5/15 minute load averages and system uptime.
type Load struct{}

func (Load) Name() string { return "load" }

func (Load) Collect(time.Time) ([]Sample, error) {
	loadavg, err := readFields("/proc/loadavg", 3)
	if err != nil {
		return nil, err
	}
	uptime, err := readFields("/proc/uptime", 1)
	if err != nil {
		return nil, err
	}

	out := make([]Sample, 0, 4)
	for i, metric := range []string{"load.1m", "load.5m", "load.15m"} {
		v, err := strconv.ParseFloat(loadavg[i], 64)
		if err != nil {
			return nil, fmt.Errorf("parse /proc/loadavg: %w", err)
		}
		out = append(out, Sample{Metric: metric, Value: v})
	}

	up, err := strconv.ParseFloat(uptime[0], 64)
	if err != nil {
		return nil, fmt.Errorf("parse /proc/uptime: %w", err)
	}
	return append(out, Sample{Metric: "uptime.seconds", Value: up}), nil
}

func readFields(path string, min int) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f := strings.Fields(string(data))
	if len(f) < min {
		return nil, fmt.Errorf("%s: expected at least %d fields, got %d", path, min, len(f))
	}
	return f, nil
}
