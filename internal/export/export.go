// Package export delivers collected metrics to external systems. Samples
// stay in metricsd's own format until an Exporter translates them, so new
// destinations are added without touching collectors or storage.
package export

import (
	"context"
	"log/slog"
	"time"

	"github.com/brian-nunez/metricsd/internal/store"
)

type Exporter interface {
	// Name identifies the exporter's delivery cursor; changing it restarts
	// delivery from the present.
	Name() string
	Export(ctx context.Context, cycles []store.Cycle) error
	Shutdown(ctx context.Context) error
}

const (
	batchCycles     = 60
	maxBackoff      = 5 * time.Minute
	shutdownTimeout = 5 * time.Second
)

// Run exports new cycles every interval until ctx is cancelled, then
// makes one last delivery attempt and shuts the exporter down.
//
// The store is the buffer: a cursor records the last delivered cycle, so
// data collected while the endpoint is unreachable, or while metricsd is
// stopped, is sent once delivery recovers, as far back as retention keeps.
func Run(ctx context.Context, exp Exporter, st *store.Store, interval time.Duration, log *slog.Logger) {
	log = log.With("exporter", exp.Name())

	cursor, err := startCursor(ctx, exp, st)
	if err != nil {
		log.Error("read export cursor", "err", err)
		return
	}

	failures := 0
	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			flushCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			if _, err := drain(flushCtx, exp, st, &cursor); err != nil {
				log.Warn("final export failed; undelivered data stays buffered", "err", err)
			}
			if err := exp.Shutdown(flushCtx); err != nil {
				log.Warn("exporter shutdown", "err", err)
			}
			cancel()
			return
		case <-timer.C:
		}

		wait := interval
		if _, err := drain(ctx, exp, st, &cursor); err != nil {
			if ctx.Err() != nil {
				// The timer stays stopped, so the next select takes the shutdown path.
				continue
			}
			failures++
			wait = backoff(interval, failures)
			if failures == 1 || failures%10 == 0 {
				log.Warn("export failed; data is buffered locally", "err", err, "failures", failures, "retry_in", wait)
			}
		} else if failures > 0 {
			log.Info("export recovered", "failures", failures)
			failures = 0
		}
		timer.Reset(wait)
	}
}

// startCursor resumes from the stored cursor. A new exporter starts at
// the present rather than replaying all retained history.
func startCursor(ctx context.Context, exp Exporter, st *store.Store) (int64, error) {
	ts, found, err := st.Cursor(ctx, exp.Name())
	if err != nil || found {
		return ts, err
	}
	ts = time.Now().Unix() - 1
	return ts, st.SetCursor(ctx, exp.Name(), ts)
}

// drain sends batches until caught up, advancing the cursor after each
// delivered batch so a failure never resends what already arrived.
func drain(ctx context.Context, exp Exporter, st *store.Store, cursor *int64) (int, error) {
	sent := 0
	for {
		cycles, err := st.Cycles(ctx, *cursor, batchCycles)
		if err != nil || len(cycles) == 0 {
			return sent, err
		}
		if err := exp.Export(ctx, cycles); err != nil {
			return sent, err
		}
		*cursor = cycles[len(cycles)-1].TS
		if err := st.SetCursor(ctx, exp.Name(), *cursor); err != nil {
			return sent, err
		}
		sent += len(cycles)
		if len(cycles) < batchCycles {
			return sent, nil
		}
	}
}

func backoff(interval time.Duration, failures int) time.Duration {
	wait := interval
	for range min(failures, 16) {
		wait *= 2
		if wait >= maxBackoff {
			return maxBackoff
		}
	}
	return wait
}
