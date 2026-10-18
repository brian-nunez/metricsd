// Package wire defines the JSON types exchanged between metricsd and its
// clients. It depends only on the standard library so clients stay small.
package wire

import "strings"

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

// Info identifies the environment metricsd measures.
type Info struct {
	Environment     string            `json:"environment"`
	ID              string            `json:"id"`
	Hostname        string            `json:"hostname"`
	OS              string            `json:"os"`
	Arch            string            `json:"arch"`
	Platform        string            `json:"platform,omitempty"`
	PlatformVersion string            `json:"platform_version,omitempty"`
	KernelVersion   string            `json:"kernel_version,omitempty"`
	CPUModel        string            `json:"cpu_model,omitempty"`
	Hypervisor      string            `json:"hypervisor,omitempty"`
	Container       *ContainerInfo    `json:"container,omitempty"`
	Kubernetes      *KubernetesInfo   `json:"kubernetes,omitempty"`
	Tags            map[string]string `json:"tags,omitempty"`
	AgentVersion    string            `json:"agent_version"`
	StartedAt       int64             `json:"started_at"`
}

type ContainerInfo struct {
	Runtime string `json:"runtime,omitempty"`
	ID      string `json:"id,omitempty"`
}

type KubernetesInfo struct {
	Namespace string `json:"namespace,omitempty"`
	Pod       string `json:"pod,omitempty"`
	PodUID    string `json:"pod_uid,omitempty"`
	Container string `json:"container,omitempty"`
	Node      string `json:"node,omitempty"`
}

type Health struct {
	Status string `json:"status"`
}

type Error struct {
	Error string `json:"error"`
}

// Unit returns the UCUM unit of a metric, derived from its name suffix.
// metricsctl formatting and exporters both rely on it, so a metric with a
// new unit suffix needs a case here.
func Unit(metric string) string {
	switch {
	case strings.HasSuffix(metric, "_bytes_per_sec"):
		return "By/s"
	case strings.HasSuffix(metric, "_bytes"):
		return "By"
	case strings.HasSuffix(metric, "_pct"):
		return "%"
	case strings.HasSuffix(metric, "_per_sec"):
		return "1/s"
	case strings.HasSuffix(metric, "_mhz"):
		return "MHz"
	case strings.HasSuffix(metric, "_ms"):
		return "ms"
	case strings.HasSuffix(metric, "_watts"):
		return "W"
	case strings.HasSuffix(metric, ".celsius"):
		return "Cel"
	case strings.HasSuffix(metric, ".seconds"):
		return "s"
	case strings.HasSuffix(metric, ".rpm"):
		return "1/min"
	default:
		return "1"
	}
}
