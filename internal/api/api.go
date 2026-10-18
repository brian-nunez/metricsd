package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/brian-nunez/metricsd/internal/config"
	"github.com/brian-nunez/metricsd/internal/store"
	"github.com/brian-nunez/metricsd/internal/wire"
)

const (
	defaultSince  = time.Hour
	maxSince      = 366 * 24 * time.Hour
	defaultPoints = 120
	maxPoints     = 10000
	queryTimeout  = 20 * time.Second
)

type handler struct {
	store *store.Store
	info  wire.Info
	log   *slog.Logger
}

func NewHandler(st *store.Store, info wire.Info, log *slog.Logger) http.Handler {
	h := &handler{store: st, info: info, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", h.health)
	mux.HandleFunc("GET /v1/info", h.identity)
	mux.HandleFunc("GET /v1/metrics", h.latest)
	mux.HandleFunc("GET /v1/metrics/{name}", h.history)
	return mux
}

func (h *handler) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), queryTimeout)
	defer cancel()

	if err := h.store.Ping(ctx); err != nil {
		h.fail(w, http.StatusServiceUnavailable, "database unavailable", err)
		return
	}
	writeJSON(w, http.StatusOK, wire.Health{Status: "ok"})
}

func (h *handler) identity(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.info)
}

func (h *handler) latest(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), queryTimeout)
	defer cancel()

	ts, samples, err := h.store.Latest(ctx)
	if err != nil {
		h.fail(w, http.StatusInternalServerError, "query latest samples", err)
		return
	}

	snap := wire.Snapshot{TS: ts, Samples: make([]wire.Sample, len(samples))}
	for i, s := range samples {
		snap.Samples[i] = wire.Sample{Metric: s.Metric, Labels: s.Labels, Value: s.Value}
	}
	writeJSON(w, http.StatusOK, snap)
}

func (h *handler) history(w http.ResponseWriter, r *http.Request) {
	metric := r.PathValue("name")
	q := r.URL.Query()

	since := defaultSince
	if v := q.Get("since"); v != "" {
		d, err := config.ParseDuration(v)
		if err != nil || d <= 0 || d > maxSince {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("since must be a duration between 1s and %s", maxSince))
			return
		}
		since = d
	}

	step := max((since / defaultPoints).Truncate(time.Second), time.Second)
	if v := q.Get("step"); v != "" {
		d, err := config.ParseDuration(v)
		if err != nil || d < time.Second {
			writeError(w, http.StatusBadRequest, "step must be a duration of at least 1s")
			return
		}
		step = d.Truncate(time.Second)
	}
	if since/step > maxPoints {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("step too small: range would exceed %d points", maxPoints))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), queryTimeout)
	defer cancel()

	from := time.Now().Add(-since)
	series, err := h.store.History(ctx, metric, from, step)
	if err != nil {
		h.fail(w, http.StatusInternalServerError, "query history", err)
		return
	}
	if len(series) == 0 {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no data for metric %q in the last %s", metric, since))
		return
	}

	out := wire.History{
		Metric:      metric,
		Since:       from.Unix(),
		StepSeconds: int64(step / time.Second),
		Series:      make([]wire.Series, len(series)),
	}
	for i, s := range series {
		points := make([]wire.Point, len(s.Points))
		for j, p := range s.Points {
			points[j] = wire.Point{TS: p.TS, Value: p.Value}
		}
		out.Series[i] = wire.Series{Labels: s.Labels, Points: points}
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) fail(w http.ResponseWriter, status int, msg string, err error) {
	h.log.Error(msg, "err", err)
	writeError(w, status, msg)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, wire.Error{Error: msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// Listen opens the Unix socket at path, replacing a stale socket left by a
// crashed instance but refusing to start if another instance is live.
func Listen(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create socket dir: %w", err)
	}

	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("%s exists and is not a socket", path)
		}
		if conn, err := net.DialTimeout("unix", path, time.Second); err == nil {
			conn.Close()
			return nil, fmt.Errorf("%s: another metricsd is already running", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("remove stale socket: %w", err)
		}
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, fmt.Errorf("chmod socket: %w", err)
	}
	return ln, nil
}
