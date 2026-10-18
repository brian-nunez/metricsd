package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/brian-nunez/metricsd/internal/api"
	"github.com/brian-nunez/metricsd/internal/collector"
	"github.com/brian-nunez/metricsd/internal/config"
	"github.com/brian-nunez/metricsd/internal/store"
)

func main() {
	configPath := flag.String("config", config.DefaultConfigPath(), "path to the TOML config file")
	flag.Parse()

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
		Handler:           api.NewHandler(st, log),
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

	collectors := buildCollectors(cfg)
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
		"db", cfg.DBPath,
		"socket", cfg.SocketPath,
		"interval", interval,
		"retention", retention,
		"collectors", len(collectors),
	)

	// Rate-based collectors need a baseline before they can report.
	collect(time.Now())
	prune(time.Now())

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

func buildCollectors(cfg config.Config) []collector.Collector {
	available := map[string]collector.Collector{
		"cpu":    &collector.CPU{},
		"mem":    collector.Mem{},
		"load":   collector.Load{},
		"disk":   collector.Disk{},
		"diskio": &collector.DiskIO{Exclude: cfg.DiskIO.ExcludeDevices},
		"net":    &collector.Net{Exclude: cfg.Net.ExcludeInterfaces},
		"temp":   collector.Temp{},
	}

	var out []collector.Collector
	for _, name := range config.CollectorNames {
		if cfg.Collectors[name] {
			out = append(out, available[name])
		}
	}
	return out
}
