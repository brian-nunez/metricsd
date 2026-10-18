package store

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/brian-nunez/metricsd/internal/collector"
)

// Series metadata is stored once; points reference it by id. This keeps
// each point to a few bytes instead of repeating metric and label text.
const schema = `
CREATE TABLE IF NOT EXISTS series (
	id     INTEGER PRIMARY KEY,
	metric TEXT NOT NULL,
	labels TEXT NOT NULL,
	UNIQUE (metric, labels)
);
CREATE TABLE IF NOT EXISTS points (
	series_id INTEGER NOT NULL REFERENCES series (id),
	ts        INTEGER NOT NULL,
	value     REAL    NOT NULL,
	PRIMARY KEY (series_id, ts)
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS points_ts ON points (ts);
`

type Point struct {
	TS    int64
	Value float64
}

type Series struct {
	Labels string
	Points []Point
}

type seriesKey struct {
	metric, labels string
}

type Store struct {
	w *sql.DB // single writer connection
	r *sql.DB // read-only pool, so API queries never block collection

	mu     sync.Mutex
	series map[seriesKey]int64
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	base := "file:" + path + "?_pragma=busy_timeout(5000)"

	w, err := sql.Open("sqlite", base+"&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	w.SetMaxOpenConns(1)
	if _, err := w.Exec(schema); err != nil {
		w.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	r, err := sql.Open("sqlite", base+"&_pragma=query_only(1)")
	if err != nil {
		w.Close()
		return nil, err
	}
	r.SetMaxOpenConns(4)

	return &Store{w: w, r: r, series: make(map[seriesKey]int64)}, nil
}

// Insert writes all samples for one collection cycle in a single transaction.
func (s *Store) Insert(ctx context.Context, ts time.Time, samples []collector.Sample) error {
	if len(samples) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.w.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// DO UPDATE is a no-op write that makes RETURNING yield the existing id.
	upsert, err := tx.PrepareContext(ctx, `
		INSERT INTO series (metric, labels) VALUES (?, ?)
		ON CONFLICT (metric, labels) DO UPDATE SET metric = excluded.metric
		RETURNING id`)
	if err != nil {
		return err
	}
	defer upsert.Close()

	insert, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO points (series_id, ts, value) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insert.Close()

	// New ids join the cache only after commit, so a rollback can't leave
	// it pointing at rows that don't exist.
	created := make(map[seriesKey]int64)
	unix := ts.Unix()
	for _, smp := range samples {
		key := seriesKey{smp.Metric, smp.Labels}
		id, ok := s.series[key]
		if !ok {
			id, ok = created[key]
		}
		if !ok {
			if err := upsert.QueryRowContext(ctx, smp.Metric, smp.Labels).Scan(&id); err != nil {
				return fmt.Errorf("resolve series %s{%s}: %w", smp.Metric, smp.Labels, err)
			}
			created[key] = id
		}
		if _, err := insert.ExecContext(ctx, id, unix, smp.Value); err != nil {
			return fmt.Errorf("insert %s{%s}: %w", smp.Metric, smp.Labels, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	maps.Copy(s.series, created)
	return nil
}

// Prune deletes points older than cutoff, then any series left without
// points (e.g. a removed network interface). It returns the points removed.
func (s *Store) Prune(ctx context.Context, cutoff time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.w.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `DELETE FROM points WHERE ts < ?`, cutoff.Unix())
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM series
		WHERE NOT EXISTS (SELECT 1 FROM points WHERE points.series_id = series.id)`); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	clear(s.series)
	return n, nil
}

// Latest returns every sample from the most recent collection cycle.
func (s *Store) Latest(ctx context.Context) (int64, []collector.Sample, error) {
	rows, err := s.r.QueryContext(ctx, `
		SELECT p.ts, s.metric, s.labels, p.value
		FROM points p JOIN series s ON s.id = p.series_id
		WHERE p.ts = (SELECT max(ts) FROM points)
		ORDER BY s.metric, s.labels`)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()

	var ts int64
	var out []collector.Sample
	for rows.Next() {
		var smp collector.Sample
		if err := rows.Scan(&ts, &smp.Metric, &smp.Labels, &smp.Value); err != nil {
			return 0, nil, err
		}
		out = append(out, smp)
	}
	return ts, out, rows.Err()
}

// History returns one metric's values since the given time, averaged into
// step-wide buckets aligned to the Unix epoch.
func (s *Store) History(ctx context.Context, metric string, since time.Time, step time.Duration) ([]Series, error) {
	stepSec := int64(step / time.Second)
	if stepSec < 1 {
		return nil, fmt.Errorf("step must be at least 1s")
	}

	rows, err := s.r.QueryContext(ctx, `
		SELECT s.labels, (p.ts / ?1) * ?1 AS bucket, avg(p.value)
		FROM series s JOIN points p ON p.series_id = s.id
		WHERE s.metric = ?2 AND p.ts >= ?3
		GROUP BY s.labels, bucket
		ORDER BY s.labels, bucket`, stepSec, metric, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Series
	for rows.Next() {
		var labels string
		var p Point
		if err := rows.Scan(&labels, &p.TS, &p.Value); err != nil {
			return nil, err
		}
		if len(out) == 0 || out[len(out)-1].Labels != labels {
			out = append(out, Series{Labels: labels})
		}
		last := &out[len(out)-1]
		last.Points = append(last.Points, p)
	}
	return out, rows.Err()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.r.PingContext(ctx)
}

func (s *Store) Close() error {
	rerr := s.r.Close()
	if err := s.w.Close(); err != nil {
		return err
	}
	return rerr
}
