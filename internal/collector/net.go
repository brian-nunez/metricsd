package collector

import (
	"time"

	psnet "github.com/shirou/gopsutil/v4/net"
)

// Net reports throughput, packet, error, and drop rates per network
// interface. /proc/net/dev is scoped to the network namespace, so inside a
// container it lists the container's own interfaces. Interfaces matching
// any Exclude glob are skipped.
type Net struct {
	Exclude []string

	counters deltas
}

func (n *Net) Name() string { return "net" }

func (n *Net) Collect(now time.Time) ([]Sample, error) {
	stats, err := psnet.IOCounters(true)
	if err != nil {
		return nil, err
	}

	cur := make(map[series]float64, len(stats)*8)
	for _, s := range stats {
		if matchAny(n.Exclude, s.Name) {
			continue
		}
		l := labels("iface", s.Name)
		cur[series{"net.rx_bytes_per_sec", l}] = float64(s.BytesRecv)
		cur[series{"net.tx_bytes_per_sec", l}] = float64(s.BytesSent)
		cur[series{"net.rx_packets_per_sec", l}] = float64(s.PacketsRecv)
		cur[series{"net.tx_packets_per_sec", l}] = float64(s.PacketsSent)
		cur[series{"net.rx_errors_per_sec", l}] = float64(s.Errin)
		cur[series{"net.tx_errors_per_sec", l}] = float64(s.Errout)
		cur[series{"net.rx_drops_per_sec", l}] = float64(s.Dropin)
		cur[series{"net.tx_drops_per_sec", l}] = float64(s.Dropout)
	}
	return n.counters.rates(now, cur), nil
}
