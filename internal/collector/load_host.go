package collector

import (
	"time"

	"github.com/shirou/gopsutil/v4/load"
)

// HostLoad reports the 1/5/15 minute load averages. Load is a
// machine-wide kernel figure, so containers don't report it.
type HostLoad struct{}

func (HostLoad) Name() string { return "load" }

func (HostLoad) Collect(time.Time) ([]Sample, error) {
	avg, err := load.Avg()
	if err != nil {
		return nil, err
	}
	return []Sample{
		gauge("load.1m", "", avg.Load1),
		gauge("load.5m", "", avg.Load5),
		gauge("load.15m", "", avg.Load15),
	}, nil
}
