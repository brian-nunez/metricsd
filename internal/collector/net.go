package collector

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Net reports receive/transmit throughput per network interface.
// Interfaces matching any Exclude glob are skipped.
type Net struct {
	Exclude []string

	rate counterRate
}

func (n *Net) Name() string { return "net" }

func (n *Net) Collect(now time.Time) ([]Sample, error) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil, err
	}

	cur := make(map[string]uint64)
	for _, line := range strings.Split(string(data), "\n") {
		iface, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		iface = strings.TrimSpace(iface)
		if matchAny(n.Exclude, iface) {
			continue
		}
		// 8 receive columns followed by 8 transmit columns; bytes is first in each.
		f := strings.Fields(rest)
		if len(f) < 16 {
			continue
		}
		rx, err1 := strconv.ParseUint(f[0], 10, 64)
		tx, err2 := strconv.ParseUint(f[8], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		cur["rx|"+iface] = rx
		cur["tx|"+iface] = tx
	}

	rates := n.rate.update(now, cur)
	out := make([]Sample, 0, len(rates))
	for key, v := range rates {
		dir, iface, _ := strings.Cut(key, "|")
		out = append(out, Sample{Metric: "net." + dir + "_bytes_per_sec", Labels: "iface=" + iface, Value: v})
	}
	return out, nil
}
