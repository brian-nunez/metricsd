package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/brian-nunez/metricsd/internal/wire"
)

// formatValue renders v in the metric's unit.
func formatValue(metric string, v float64) string {
	switch wire.Unit(metric) {
	case "By/s":
		return formatBytes(v) + "/s"
	case "By":
		return formatBytes(v)
	case "%":
		return fmt.Sprintf("%.1f%%", v)
	case "1/s":
		return fmt.Sprintf("%.1f/s", v)
	case "MHz":
		return fmt.Sprintf("%.0f MHz", v)
	case "ms":
		return fmt.Sprintf("%.2f ms", v)
	case "W":
		return fmt.Sprintf("%.1f W", v)
	case "Cel":
		return fmt.Sprintf("%.1f°C", v)
	case "s":
		return formatUptime(time.Duration(v * float64(time.Second)))
	case "1/min":
		return fmt.Sprintf("%.0f RPM", v)
	default:
		if v == math.Trunc(v) && math.Abs(v) < 1e15 {
			return strconv.FormatFloat(v, 'f', 0, 64)
		}
		return strconv.FormatFloat(v, 'f', 2, 64)
	}
}

func formatBytes(v float64) string {
	const unit = 1024
	if v < unit {
		return fmt.Sprintf("%.0f B", v)
	}
	const prefixes = "KMGTPE"
	i := -1
	for v >= unit && i < len(prefixes)-1 {
		v /= unit
		i++
	}
	return fmt.Sprintf("%.1f %ciB", v, prefixes[i])
}

func formatUptime(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Second).String()
	}
	d = d.Round(time.Minute)
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, d/time.Minute)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, d/time.Minute)
	}
	return fmt.Sprintf("%dm", d/time.Minute)
}

func printJSONValue(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// naturalCompare orders strings with embedded numbers numerically, so
// "core=2" sorts before "core=10".
func naturalCompare(a, b string) int {
	for a != "" && b != "" {
		if isDigit(a[0]) && isDigit(b[0]) {
			na, ra := splitDigits(a)
			nb, rb := splitDigits(b)
			na, nb = strings.TrimLeft(na, "0"), strings.TrimLeft(nb, "0")
			if len(na) != len(nb) {
				return len(na) - len(nb)
			}
			if c := strings.Compare(na, nb); c != 0 {
				return c
			}
			a, b = ra, rb
			continue
		}
		if a[0] != b[0] {
			return int(a[0]) - int(b[0])
		}
		a, b = a[1:], b[1:]
	}
	return len(a) - len(b)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func splitDigits(s string) (digits, rest string) {
	i := 0
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	return s[:i], s[i:]
}
