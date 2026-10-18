package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// CollectorNames lists every collector in the order they run.
var CollectorNames = []string{"cpu", "mem", "load", "disk", "diskio", "net", "temp"}

type Config struct {
	Interval   Duration        `toml:"interval"`
	Retention  Duration        `toml:"retention"`
	DBPath     string          `toml:"db_path"`
	SocketPath string          `toml:"socket_path"`
	Collectors map[string]bool `toml:"collectors"`
	Net        NetConfig       `toml:"net"`
	DiskIO     DiskIOConfig    `toml:"diskio"`
}

type NetConfig struct {
	ExcludeInterfaces []string `toml:"exclude_interfaces"`
}

type DiskIOConfig struct {
	ExcludeDevices []string `toml:"exclude_devices"`
}

func Default() Config {
	collectors := make(map[string]bool, len(CollectorNames))
	for _, name := range CollectorNames {
		collectors[name] = true
	}
	return Config{
		Interval:   Duration(5 * time.Second),
		Retention:  Duration(7 * 24 * time.Hour),
		DBPath:     DefaultDBPath(),
		SocketPath: DefaultSocketPath(),
		Collectors: collectors,
		Net: NetConfig{
			ExcludeInterfaces: []string{"lo", "veth*", "br-*", "docker*"},
		},
		DiskIO: DiskIOConfig{
			// dm-* devices are device-mapper layers (LUKS, LVM) that mirror
			// the I/O of the physical disk underneath them.
			ExcludeDevices: []string{"loop*", "ram*", "dm-*"},
		},
	}
}

// Load reads a TOML config file on top of the defaults. Keys absent from
// the file keep their default values; lists replace the defaults entirely.
func Load(file string) (Config, error) {
	cfg := Default()
	md, err := toml.DecodeFile(file, &cfg)
	if err != nil {
		return Config{}, err
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return Config{}, fmt.Errorf("%s: unknown keys: %v", file, undecoded)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", file, err)
	}
	return cfg, nil
}

func (c *Config) Validate() error {
	if c.Interval.D() < time.Second {
		return fmt.Errorf("interval must be at least 1s, got %s", c.Interval.D())
	}
	if c.Retention.D() < c.Interval.D() {
		return fmt.Errorf("retention (%s) must be at least the interval (%s)", c.Retention.D(), c.Interval.D())
	}

	var err error
	if c.DBPath, err = expandPath(c.DBPath, "db_path"); err != nil {
		return err
	}
	if c.SocketPath, err = expandPath(c.SocketPath, "socket_path"); err != nil {
		return err
	}
	// sun_path is 108 bytes including the trailing NUL.
	if len(c.SocketPath) > 107 {
		return fmt.Errorf("socket_path is %d bytes; Unix sockets allow at most 107", len(c.SocketPath))
	}

	known := make(map[string]bool, len(CollectorNames))
	for _, name := range CollectorNames {
		known[name] = true
	}
	for name := range c.Collectors {
		if !known[name] {
			return fmt.Errorf("unknown collector %q (valid: %s)", name, strings.Join(CollectorNames, ", "))
		}
	}

	for _, patterns := range [][]string{c.Net.ExcludeInterfaces, c.DiskIO.ExcludeDevices} {
		for _, p := range patterns {
			if _, err := path.Match(p, ""); err != nil {
				return fmt.Errorf("invalid exclude pattern %q: %w", p, err)
			}
		}
	}
	return nil
}

func expandPath(p, key string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("%s must not be empty", key)
	}
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("%s: %w", key, err)
		}
		p = filepath.Join(home, rest)
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("%s must be an absolute path, got %q", key, p)
	}
	return filepath.Clean(p), nil
}

func DefaultConfigPath() string {
	return filepath.Join(xdgDir("XDG_CONFIG_HOME", ".config"), "metricsd", "config.toml")
}

func DefaultDBPath() string {
	// Set by systemd when the unit declares StateDirectory=.
	if dir := os.Getenv("STATE_DIRECTORY"); dir != "" {
		return filepath.Join(dir, "metrics.db")
	}
	return filepath.Join(xdgDir("XDG_STATE_HOME", ".local/state"), "metricsd", "metrics.db")
}

func DefaultSocketPath() string {
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	if runtime == "" {
		runtime = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	return filepath.Join(runtime, "metricsd", "metricsd.sock")
}

func xdgDir(env, fallback string) string {
	if dir := os.Getenv(env); filepath.IsAbs(dir) {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fallback
	}
	return filepath.Join(home, fallback)
}

// Duration is a time.Duration that decodes from strings such as "5s",
// "12h", or "7d".
type Duration time.Duration

func (d Duration) D() time.Duration { return time.Duration(d) }

func (d *Duration) UnmarshalText(b []byte) error {
	v, err := ParseDuration(string(b))
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// ParseDuration extends time.ParseDuration with a standalone day unit
// ("7d"). Mixed forms like "1d12h" are not supported; use "36h".
func ParseDuration(s string) (time.Duration, error) {
	if n, ok := strings.CutSuffix(s, "d"); ok {
		days, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		return time.Duration(days * float64(24*time.Hour)), nil
	}
	return time.ParseDuration(s)
}
