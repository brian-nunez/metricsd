package collector

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Temp reports every hwmon temperature sensor in degrees Celsius.
type Temp struct{}

func (Temp) Name() string { return "temp" }

func (Temp) Collect(time.Time) ([]Sample, error) {
	inputs, err := filepath.Glob("/sys/class/hwmon/hwmon*/temp*_input")
	if err != nil {
		return nil, err
	}

	out := make([]Sample, 0, len(inputs))
	for _, input := range inputs {
		raw, err := os.ReadFile(input)
		if err != nil {
			// Some sensors return EIO while their device is suspended.
			continue
		}
		milli, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
		if err != nil {
			continue
		}

		dir := filepath.Dir(input)
		chip := readTrimmed(filepath.Join(dir, "name"))
		if chip == "" {
			chip = filepath.Base(dir)
		}
		sensor := readTrimmed(strings.TrimSuffix(input, "_input") + "_label")
		if sensor == "" {
			sensor = strings.TrimSuffix(filepath.Base(input), "_input")
		}

		labels := "chip=" + chip
		// Identical chips (e.g. one per DIMM) share a name; the device address
		// tells them apart and, unlike hwmonN, is stable across boots.
		if dev, err := filepath.EvalSymlinks(filepath.Join(dir, "device")); err == nil {
			labels += ",device=" + filepath.Base(dev)
		}
		labels += ",sensor=" + sensor

		out = append(out, Sample{
			Metric: "temp.celsius",
			Labels: labels,
			Value:  milli / 1000,
		})
	}
	return out, nil
}

func readTrimmed(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
