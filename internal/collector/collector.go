package collector

import (
	"os"
	"path"
	"strconv"
	"strings"
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

// Checker is implemented by collectors whose data source may be absent in
// some environments (no sensors, a cgroup controller not delegated to a
// rootless container). Check runs once at startup; an error disables the
// collector instead of logging the same failure every cycle.
type Checker interface {
	Check() error
}

var labelEscaper = strings.NewReplacer(",", "_", "=", "_")

// labels builds a label string from key/value pairs. Values are sanitized
// so exporters can split the string back into pairs unambiguously.
func labels(kv ...string) string {
	var b strings.Builder
	for i := 0; i+1 < len(kv); i += 2 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(kv[i])
		b.WriteByte('=')
		b.WriteString(labelEscaper.Replace(kv[i+1]))
	}
	return b.String()
}

func gauge(metric, labels string, v float64) Sample {
	return Sample{Metric: metric, Labels: labels, Value: v}
}

func pct(part, whole float64) float64 {
	if whole <= 0 {
		return 0
	}
	return part / whole * 100
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

type series struct {
	metric, labels string
}

// deltas tracks monotonically increasing counters between collection
// cycles. The first update records a baseline and reports ok=false.
// Counters that went backwards (device re-added, counter reset) are left
// out of the result for one cycle.
type deltas struct {
	prev     map[series]float64
	prevTime time.Time
}

func (d *deltas) update(now time.Time, cur map[series]float64) (diff map[series]float64, secs float64, ok bool) {
	defer func() { d.prev, d.prevTime = cur, now }()

	if d.prev == nil {
		return nil, 0, false
	}
	secs = now.Sub(d.prevTime).Seconds()
	if secs <= 0 {
		return nil, 0, false
	}

	diff = make(map[series]float64, len(cur))
	for s, v := range cur {
		p, found := d.prev[s]
		if !found || v < p {
			continue
		}
		diff[s] = v - p
	}
	return diff, secs, true
}

// rates returns each counter's per-second increase, keyed by the output
// metric and labels the caller used.
func (d *deltas) rates(now time.Time, cur map[series]float64) []Sample {
	diff, secs, ok := d.update(now, cur)
	if !ok {
		return nil
	}
	out := make([]Sample, 0, len(diff))
	for s, v := range diff {
		out = append(out, gauge(s.metric, s.labels, v/secs))
	}
	return out
}

func readTrimmed(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readFloat(path string) (float64, bool) {
	s := readTrimmed(path)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
