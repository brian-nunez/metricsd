// Package wire defines the JSON types exchanged between metricsd and its
// clients. It has no dependencies so clients stay small.
package wire

type Sample struct {
	Metric string  `json:"metric"`
	Labels string  `json:"labels,omitempty"`
	Value  float64 `json:"value"`
}

// Snapshot is the most recent collection cycle.
type Snapshot struct {
	TS      int64    `json:"ts"`
	Samples []Sample `json:"samples"`
}

type Point struct {
	TS    int64   `json:"ts"`
	Value float64 `json:"value"`
}

type Series struct {
	Labels string  `json:"labels,omitempty"`
	Points []Point `json:"points"`
}

// History is one metric's values averaged into fixed-width time buckets.
type History struct {
	Metric      string   `json:"metric"`
	Since       int64    `json:"since"`
	StepSeconds int64    `json:"step_seconds"`
	Series      []Series `json:"series"`
}

type Health struct {
	Status string `json:"status"`
}

type Error struct {
	Error string `json:"error"`
}
