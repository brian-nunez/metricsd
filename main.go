package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/brian-nunez/metricsd/internal/api"
	"github.com/brian-nunez/metricsd/internal/collector"
	"github.com/brian-nunez/metricsd/internal/config"
	"github.com/brian-nunez/metricsd/internal/environment"
	"github.com/brian-nunez/metricsd/internal/export"
	"github.com/brian-nunez/metricsd/internal/export/otlp"
	"github.com/brian-nunez/metricsd/internal/identity"
	"github.com/brian-nunez/metricsd/internal/store"
	"github.com/brian-nunez/metricsd/internal/version"
)

func main() {
	configPath := flag.String("config", config.DefaultConfigPath(), "path to the TOML config file")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.String())
		return
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	explicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "config" {
			explicit = true
		}
	})

	cfg, err := loadConfig(*configPath, explicit)
	if err != nil {
		log.Error("load config", "err", err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, log, cfg); err != nil {
		log.Error("metricsd exited", "err", err)
		os.Exit(1)
	}
}

// loadConfig falls back to defaults when the default config file is
// missing, but treats a missing file passed via -config as an error.
func loadConfig(path string, required bool) (config.Config, error) {
	cfg, err := config.Load(path)
	if errors.Is(err, fs.ErrNotExist) && !required {
		cfg = config.Default()
		return cfg, cfg.Validate()
	}
	return cfg, err
}

func run(ctx context.Context, log *slog.Logger, cfg config.Config) error {
	env := cfg.ResolveEnvironment()
	info := identity.Gather(ctx, env, cfg.Tags)

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	ln, err := api.Listen(cfg.SocketPath)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Handler:           api.NewHandler(st, info, log),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Warn("api shutdown", "err", err)
		}
	}()

	collectors := buildCollectors(cfg, env, log)
	interval, retention := cfg.Interval.D(), cfg.Retention.D()

	collect := func(now time.Time) {
		var samples []collector.Sample
		for _, c := range collectors {
			s, err := c.Collect(now)
			if err != nil {
				log.Warn("collect failed", "collector", c.Name(), "err", err)
				continue
			}
			samples = append(samples, s...)
		}
		if err := st.Insert(ctx, now, samples); err != nil && ctx.Err() == nil {
			log.Error("store samples", "err", err)
		}
	}

	prune := func(now time.Time) {
		n, err := st.Prune(ctx, now.Add(-retention))
		if err != nil {
			if ctx.Err() == nil {
				log.Error("prune samples", "err", err)
			}
			return
		}
		if n > 0 {
			log.Info("pruned old samples", "points", n)
		}
	}

	log.Info("metricsd started",
		"version", info.AgentVersion,
		"environment", env,
		"id", info.ID,
		"db", cfg.DBPath,
		"socket", cfg.SocketPath,
		"interval", interval,
		"retention", retention,
		"collectors", len(collectors),
	)

	// Rate-based collectors need a baseline before they can report.
	collect(time.Now())
	prune(time.Now())

	// The exporter reads from the store, so it must finish its final
	// delivery before the deferred st.Close runs.
	exportDone := make(chan struct{})
	close(exportDone)
	if cfg.Export.OTLP.Enabled {
		exp, err := otlp.New(ctx, cfg.Export.OTLP, info)
		if err != nil {
			return err
		}
		exportDone = make(chan struct{})
		go func() {
			defer close(exportDone)
			export.Run(ctx, exp, st, cfg.Export.OTLP.Interval.D(), log)
		}()
		log.Info("otlp export enabled",
			"protocol", cfg.Export.OTLP.Protocol,
			"endpoint", otlpEndpointForLog(cfg.Export.OTLP.Endpoint),
			"interval", cfg.Export.OTLP.Interval.D(),
		)
	}
	defer func() { <-exportDone }()

	collectTick := time.NewTicker(interval)
	defer collectTick.Stop()
	pruneTick := time.NewTicker(time.Hour)
	defer pruneTick.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("metricsd stopping")
			return nil
		case err := <-serveErr:
			return fmt.Errorf("api server: %w", err)
		case now := <-collectTick.C:
			collect(now)
		case now := <-pruneTick.C:
			prune(now)
		}
	}
}

// buildCollectors picks the implementation of each collector for the
// environment. Containers read their own cgroup instead of machine-wide
// kernel files, and have no load average or hardware sensors of their own.
func buildCollectors(cfg config.Config, env environment.Kind, log *slog.Logger) []collector.Collector {
	available := map[string]collector.Collector{
		"disk":    collector.Disk{Container: env.IsContainer()},
		"net":     &collector.Net{Exclude: cfg.Net.ExcludeInterfaces},
		"netstat": &collector.NetStat{Container: env.IsContainer()},
	}
	unavailable := make(map[string]string)

	if env.IsContainer() {
		cg, err := collector.FindCgroup()
		if err != nil {
			for _, name := range []string{"cpu", "mem", "diskio", "pressure"} {
				unavailable[name] = err.Error()
			}
			available["system"] = &collector.ContainerSystem{}
		} else {
			available["cpu"] = &collector.ContainerCPU{Cgroup: cg}
			available["mem"] = &collector.ContainerMem{Cgroup: cg}
			available["diskio"] = &collector.ContainerDiskIO{Cgroup: cg, Exclude: cfg.DiskIO.ExcludeDevices}
			available["pressure"] = collector.CgroupPressure(cg)
			available["system"] = &collector.ContainerSystem{Cgroup: cg}
		}
		for _, name := range []string{"load", "temp", "fan", "battery"} {
			unavailable[name] = "machine-wide only; not measured inside containers"
		}
	} else {
		available["cpu"] = &collector.HostCPU{}
		available["mem"] = &collector.HostMem{}
		available["load"] = collector.HostLoad{}
		available["diskio"] = &collector.HostDiskIO{Exclude: cfg.DiskIO.ExcludeDevices}
		available["system"] = &collector.HostSystem{}
		available["pressure"] = collector.HostPressure()
		available["temp"] = collector.Temp{}
		available["fan"] = collector.Fan{}
		available["battery"] = collector.Battery{}
	}

	var out []collector.Collector
	for _, name := range config.CollectorNames {
		if !cfg.Collectors[name] {
			continue
		}
		if reason, ok := unavailable[name]; ok {
			log.Info("collector skipped", "collector", name, "reason", reason)
			continue
		}
		c := available[name]
		if checker, ok := c.(collector.Checker); ok {
			if err := checker.Check(); err != nil {
				log.Info("collector skipped", "collector", name, "reason", err)
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

func otlpEndpointForLog(endpoint string) string {
	if endpoint == "" {
		return "from OTEL_EXPORTER_OTLP_* or default"
	}
	// Keep credentials embedded in the URL out of the journal.
	if u, err := url.Parse(endpoint); err == nil {
		return u.Redacted()
	}
	return endpoint
}
