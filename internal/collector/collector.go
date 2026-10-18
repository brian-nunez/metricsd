package collector

import (
	"path"
	"time"
)

// Sample is a single measured value. Labels distinguish series within a
// metric, formatted as comma-separated key=value pairs (e.g. "core=0").
type Sample struct {
	Metric string
	Labels string
	Value  float64
}

type Collector interface {
	Name() string
	Collect(now time.Time) ([]Sample, error)
}

// matchAny reports whether name matches any of the glob patterns.
// Patterns are validated at config load, so match errors are ignored.
func matchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// counterRate converts monotonically increasing counters into per-second
// rates. The first call only records a baseline and returns nil.
type counterRate struct {
	prev     map[string]uint64
	prevTime time.Time
}

func (r *counterRate) update(now time.Time, cur map[string]uint64) map[string]float64 {
	defer func() { r.prev, r.prevTime = cur, now }()

	if r.prev == nil {
		return nil
	}
	secs := now.Sub(r.prevTime).Seconds()
	if secs <= 0 {
		return nil
	}

	out := make(map[string]float64, len(cur))
	for k, v := range cur {
		p, ok := r.prev[k]
		if !ok || v < p {
			continue
		}
		out[k] = float64(v-p) / secs
	}
	return out
}
