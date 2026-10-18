package collector

import (
	"os"
	"strconv"
	"strings"
	"time"

	psnet "github.com/shirou/gopsutil/v4/net"
)

var protoRates = map[string]map[string]string{
	"tcp": {
		"ActiveOpens":  "tcp.active_opens_per_sec",
		"PassiveOpens": "tcp.passive_opens_per_sec",
		"AttemptFails": "tcp.failed_connections_per_sec",
		"EstabResets":  "tcp.resets_per_sec",
		"OutRsts":      "tcp.resets_sent_per_sec",
		"InSegs":       "tcp.segments_in_per_sec",
		"OutSegs":      "tcp.segments_out_per_sec",
		"RetransSegs":  "tcp.retransmits_per_sec",
		"InErrs":       "tcp.errors_in_per_sec",
	},
	"udp": {
		"InDatagrams":  "udp.datagrams_in_per_sec",
		"OutDatagrams": "udp.datagrams_out_per_sec",
		"InErrors":     "udp.errors_in_per_sec",
		"NoPorts":      "udp.no_port_per_sec",
		"RcvbufErrors": "udp.receive_buffer_errors_per_sec",
		"SndbufErrors": "udp.send_buffer_errors_per_sec",
	},
}

// NetStat reports TCP and UDP protocol counters and socket usage from
// /proc/net/snmp and /proc/net/sockstat, both scoped to the network
// namespace metricsd runs in.
type NetStat struct {
	// Container omits socket figures the kernel only tracks machine-wide
	// (orphans, socket buffer memory).
	Container bool

	counters deltas
}

func (n *NetStat) Name() string { return "netstat" }

func (n *NetStat) Collect(now time.Time) ([]Sample, error) {
	protos, err := psnet.ProtoCounters([]string{"tcp", "udp"})
	if err != nil {
		return nil, err
	}

	cur := make(map[series]float64)
	var out []Sample
	for _, p := range protos {
		for key, metric := range protoRates[p.Protocol] {
			if v, ok := p.Stats[key]; ok {
				cur[series{metric, ""}] = float64(v)
			}
		}
		if p.Protocol == "tcp" {
			if v, ok := p.Stats["CurrEstab"]; ok {
				out = append(out, gauge("tcp.established", "", float64(v)))
			}
		}
	}

	out = append(out, sockstat(n.Container)...)
	return append(out, n.counters.rates(now, cur)...), nil
}

// sockstat parses lines such as "TCP: inuse 12 orphan 0 tw 3 alloc 20 mem 4".
func sockstat(container bool) []Sample {
	data, err := os.ReadFile("/proc/net/sockstat")
	if err != nil {
		return nil
	}

	values := make(map[string]float64)
	for _, line := range strings.Split(string(data), "\n") {
		proto, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		f := strings.Fields(rest)
		for i := 0; i+1 < len(f); i += 2 {
			if v, err := strconv.ParseFloat(f[i+1], 64); err == nil {
				values[proto+"."+f[i]] = v
			}
		}
	}

	page := float64(os.Getpagesize())
	type metric struct {
		key, name string
		scale     float64
		hostOnly  bool
	}
	var out []Sample
	for _, m := range []metric{
		{"sockets.used", "sockets.used", 1, false},
		{"TCP.inuse", "tcp.in_use", 1, false},
		{"TCP.tw", "tcp.time_wait", 1, false},
		{"UDP.inuse", "udp.in_use", 1, false},
		{"TCP.orphan", "tcp.orphans", 1, true},
		{"TCP.alloc", "tcp.allocated", 1, true},
		{"TCP.mem", "tcp.memory_bytes", page, true},
		{"UDP.mem", "udp.memory_bytes", page, true},
	} {
		if m.hostOnly && container {
			continue
		}
		if v, ok := values[m.key]; ok {
			out = append(out, gauge(m.name, "", v*m.scale))
		}
	}
	return out
}
