package collector

import (
	"path/filepath"
	"strings"
	"time"
)

// hwmonDir is where the kernel exposes hardware monitoring chips.
var hwmonDir = "/sys/class/hwmon"

// Temp reports every hwmon temperature sensor in degrees Celsius. Sensors
// are discovered every cycle, so drivers that load late are picked up.
type Temp struct{}

func (Temp) Name() string { return "temp" }

func (Temp) Collect(time.Time) ([]Sample, error) {
	// Sensors report millidegrees.
	return readHwmon("temp", "temp.celsius", 0.001), nil
}

// Fan reports every hwmon fan speed in RPM. Machines without fan sensors
// (most laptops expose none, VMs never do) simply report nothing.
type Fan struct{}

func (Fan) Name() string { return "fan" }

func (Fan) Collect(time.Time) ([]Sample, error) {
	return readHwmon("fan", "fan.rpm", 1), nil
}

func hwmonInputs(kind string) []string {
	paths, _ := filepath.Glob(filepath.Join(hwmonDir, "hwmon*", kind+"*_input"))
	return paths
}

func readHwmon(kind, metric string, scale float64) []Sample {
	inputs := hwmonInputs(kind)
	out := make([]Sample, 0, len(inputs))
	for _, input := range inputs {
		raw, ok := readFloat(input)
		if !ok {
			// Some sensors return EIO while their device is suspended.
			continue
		}
		out = append(out, gauge(metric, hwmonLabels(input), raw*scale))
	}
	return out
}

func hwmonLabels(input string) string {
	dir := filepath.Dir(input)
	chip := readTrimmed(filepath.Join(dir, "name"))
	if chip == "" {
		chip = filepath.Base(dir)
	}
	sensor := readTrimmed(strings.TrimSuffix(input, "_input") + "_label")
	if sensor == "" {
		sensor = strings.TrimSuffix(filepath.Base(input), "_input")
	}

	kv := []string{"chip", chip}
	// Identical chips (e.g. one per DIMM) share a name; the device address
	// tells them apart and, unlike hwmonN, is stable across boots.
	if dev, err := filepath.EvalSymlinks(filepath.Join(dir, "device")); err == nil {
		kv = append(kv, "device", filepath.Base(dev))
	}
	return labels(append(kv, "sensor", sensor)...)
}
