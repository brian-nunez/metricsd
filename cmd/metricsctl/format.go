package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// formatValue renders v using the unit implied by the metric's suffix.
func formatValue(metric string, v float64) string {
	switch {
	case strings.HasSuffix(metric, "_bytes_per_sec"):
		return formatBytes(v) + "/s"
	case strings.HasSuffix(metric, "_bytes"):
		return formatBytes(v)
	case strings.HasSuffix(metric, "_pct"):
		return fmt.Sprintf("%.1f%%", v)
	case strings.HasSuffix(metric, ".celsius"):
		return fmt.Sprintf("%.1f°C", v)
	case strings.HasSuffix(metric, ".seconds"):
		return formatUptime(time.Duration(v * float64(time.Second)))
	default:
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
	d = d.Round(time.Minute)
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, d/time.Minute)
	}
	return fmt.Sprintf("%dh %dm", hours, d/time.Minute)
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
