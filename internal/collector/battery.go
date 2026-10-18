package collector

import (
	"path/filepath"
	"time"
)

var powerSupplyDir = "/sys/class/power_supply"

// Battery reports charge, power draw, and health for system batteries,
// plus whether AC power is connected. Peripheral batteries (mice,
// keyboards) are skipped. Supplies are discovered every cycle, so
// hot-swapped batteries and UPS units that appear later are picked up.
type Battery struct{}

func (Battery) Name() string { return "battery" }

func (Battery) Collect(time.Time) ([]Sample, error) {
	var out []Sample
	for _, dir := range supplies() {
		name := filepath.Base(dir)
		switch readTrimmed(filepath.Join(dir, "type")) {
		case "Mains":
			if v, ok := readFloat(filepath.Join(dir, "online")); ok {
				out = append(out, gauge("power.ac_online", labels("supply", name), v))
			}
		case "Battery":
			if isSystemBattery(dir) {
				out = append(out, batterySamples(dir, labels("battery", name))...)
			}
		}
	}
	return out, nil
}

func supplies() []string {
	dirs, _ := filepath.Glob(filepath.Join(powerSupplyDir, "*"))
	return dirs
}

func isSystemBattery(dir string) bool {
	return readTrimmed(filepath.Join(dir, "type")) == "Battery" &&
		readTrimmed(filepath.Join(dir, "scope")) != "Device"
}

func batterySamples(dir, l string) []Sample {
	read := func(file string) (float64, bool) { return readFloat(filepath.Join(dir, file)) }

	var out []Sample
	if v, ok := read("capacity"); ok {
		out = append(out, gauge("battery.charge_pct", l, v))
	}

	charging := 0.0
	if readTrimmed(filepath.Join(dir, "status")) == "Charging" {
		charging = 1
	}
	out = append(out, gauge("battery.charging", l, charging))

	// Drivers report either power directly (µW) or current (µA) and voltage (µV).
	if uw, ok := read("power_now"); ok {
		out = append(out, gauge("battery.power_watts", l, uw/1e6))
	} else if ua, ok := read("current_now"); ok {
		if uv, ok := read("voltage_now"); ok {
			out = append(out, gauge("battery.power_watts", l, ua*uv/1e12))
		}
	}

	for _, pair := range [][2]string{{"energy_full", "energy_full_design"}, {"charge_full", "charge_full_design"}} {
		full, ok1 := read(pair[0])
		design, ok2 := read(pair[1])
		if ok1 && ok2 && design > 0 {
			out = append(out, gauge("battery.health_pct", l, full/design*100))
			break
		}
	}

	if v, ok := read("cycle_count"); ok && v > 0 {
		out = append(out, gauge("battery.cycles", l, v))
	}
	return out
}
