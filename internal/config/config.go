package config

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/brian-nunez/metricsd/internal/environment"
)

// CollectorNames lists every collector in the order they run. Some only
// exist in certain environments (load and hardware sensors on machines).
var CollectorNames = []string{
	"cpu", "mem", "load", "disk", "diskio", "net", "netstat",
	"system", "pressure", "temp", "fan", "battery",
}

const EnvironmentAuto = "auto"

var reservedTagPrefixes = []string{"service.", "host.", "os.", "container.", "k8s.", "metricsd."}

type Config struct {
	Environment string            `toml:"environment"`
	Interval    Duration          `toml:"interval"`
	Retention   Duration          `toml:"retention"`
	DBPath      string            `toml:"db_path"`
	SocketPath  string            `toml:"socket_path"`
	Tags        map[string]string `toml:"tags"`
	Collectors  map[string]bool   `toml:"collectors"`
	Net         NetConfig         `toml:"net"`
	DiskIO      DiskIOConfig      `toml:"diskio"`
	Export      ExportConfig      `toml:"export"`
}

type NetConfig struct {
	ExcludeInterfaces []string `toml:"exclude_interfaces"`
}

type DiskIOConfig struct {
	ExcludeDevices []string `toml:"exclude_devices"`
}

type ExportConfig struct {
	OTLP OTLPConfig `toml:"otlp"`
}

type OTLPConfig struct {
	Enabled bool `toml:"enabled"`
	// Protocol is "http" (OTLP/HTTP protobuf) or "grpc".
	Protocol string `toml:"protocol"`
	// Endpoint is a URL; "https://" enables TLS. Empty defers to the
	// OTEL_EXPORTER_OTLP_* environment variables, then localhost.
	Endpoint    string            `toml:"endpoint"`
	Headers     map[string]string `toml:"headers"`
	Compression string            `toml:"compression"`
	Timeout     Duration          `toml:"timeout"`
	Interval    Duration          `toml:"interval"`
	CAFile      string            `toml:"ca_file"`
	CertFile    string            `toml:"cert_file"`
	KeyFile     string            `toml:"key_file"`
}

func Default() Config {
	collectors := make(map[string]bool, len(CollectorNames))
	for _, name := range CollectorNames {
		collectors[name] = true
	}
	return Config{
		Environment: EnvironmentAuto,
		Interval:    Duration(5 * time.Second),
		Retention:   Duration(24 * time.Hour),
		DBPath:      DefaultDBPath(environment.Detect().IsContainer()),
		SocketPath:  DefaultSocketPath(),
		Collectors:  collectors,
		Net: NetConfig{
			ExcludeInterfaces: []string{"lo", "veth*", "br-*", "docker*"},
		},
		DiskIO: DiskIOConfig{
			// dm-* devices are device-mapper layers (LUKS, LVM) that mirror
			// the I/O of the physical disk underneath them.
			ExcludeDevices: []string{"loop*", "ram*", "dm-*"},
		},
		Export: ExportConfig{
			OTLP: OTLPConfig{
				Protocol:    "http",
				Compression: "gzip",
				Timeout:     Duration(10 * time.Second),
				Interval:    Duration(30 * time.Second),
			},
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
	// An environment override changes where the database belongs.
	if !md.IsDefined("db_path") && cfg.Environment != EnvironmentAuto && environment.Valid(cfg.Environment) {
		cfg.DBPath = DefaultDBPath(environment.Kind(cfg.Environment).IsContainer())
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", file, err)
	}
	return cfg, nil
}

// ResolveEnvironment returns the configured environment, detecting it
// when set to "auto".
func (c *Config) ResolveEnvironment() environment.Kind {
	if c.Environment == EnvironmentAuto {
		return environment.Detect()
	}
	return environment.Kind(c.Environment)
}

func (c *Config) Validate() error {
	if c.Environment != EnvironmentAuto && !environment.Valid(c.Environment) {
		names := make([]string, len(environment.Kinds))
		for i, k := range environment.Kinds {
			names[i] = string(k)
		}
		return fmt.Errorf("invalid environment %q (valid: auto, %s)", c.Environment, strings.Join(names, ", "))
	}
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

	for k := range c.Tags {
		if strings.TrimSpace(k) == "" {
			return fmt.Errorf("tags: keys must not be empty")
		}
		// Tags become OTLP resource attributes; these namespaces carry the
		// identity metricsd detects and must not be overridden by accident.
		for _, prefix := range reservedTagPrefixes {
			if strings.HasPrefix(k, prefix) {
				return fmt.Errorf("tags: %q is reserved (keys may not start with %s)", k, strings.Join(reservedTagPrefixes, ", "))
			}
		}
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

	if c.Export.OTLP.Enabled {
		if err := c.Export.OTLP.validate(); err != nil {
			return fmt.Errorf("export.otlp: %w", err)
		}
	}
	return nil
}

func (o *OTLPConfig) validate() error {
	switch o.Protocol {
	case "http", "grpc":
	default:
		return fmt.Errorf("protocol must be \"http\" or \"grpc\", got %q", o.Protocol)
	}
	if o.Endpoint != "" {
		u, err := url.Parse(o.Endpoint)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("endpoint must be a URL like https://collector.example.com:4318, got %q", o.Endpoint)
		}
	}
	switch o.Compression {
	case "gzip", "none":
	default:
		return fmt.Errorf("compression must be \"gzip\" or \"none\", got %q", o.Compression)
	}
	if o.Timeout.D() <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	if o.Interval.D() < time.Second {
		return fmt.Errorf("interval must be at least 1s, got %s", o.Interval.D())
	}
	if (o.CertFile == "") != (o.KeyFile == "") {
		return fmt.Errorf("cert_file and key_file must be set together")
	}

	var err error
	for _, f := range []struct {
		path *string
		key  string
	}{{&o.CAFile, "ca_file"}, {&o.CertFile, "cert_file"}, {&o.KeyFile, "key_file"}} {
		if *f.path == "" {
			continue
		}
		if *f.path, err = expandPath(*f.path, f.key); err != nil {
			return err
		}
		if _, err := os.Stat(*f.path); err != nil {
			return fmt.Errorf("%s: %w", f.key, err)
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

// DefaultDBPath picks a writable location for the database. Containers
// rarely have XDG directories or a usable home, so they use /var/lib as
// root and /tmp otherwise (including rootless runtimes that map the
// container user to an unprivileged host user).
func DefaultDBPath(container bool) string {
	// Set by systemd when the unit declares StateDirectory=; several
	// directories are colon-separated, and the first is ours.
	if dirs := os.Getenv("STATE_DIRECTORY"); dirs != "" {
		dir, _, _ := strings.Cut(dirs, ":")
		return filepath.Join(dir, "metrics.db")
	}
	if container {
		if os.Geteuid() == 0 {
			return "/var/lib/metricsd/metrics.db"
		}
		return "/tmp/metricsd/metrics.db"
	}
	return filepath.Join(xdgDir("XDG_STATE_HOME", ".local/state"), "metricsd", "metrics.db")
}

// DefaultSocketPath is shared by metricsd and metricsctl, so it must not
// depend on anything only the daemon knows.
func DefaultSocketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); filepath.IsAbs(dir) {
		return filepath.Join(dir, "metricsd", "metricsd.sock")
	}
	if dir := fmt.Sprintf("/run/user/%d", os.Getuid()); isDir(dir) {
		return filepath.Join(dir, "metricsd", "metricsd.sock")
	}
	return "/tmp/metricsd/metricsd.sock"
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

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
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
