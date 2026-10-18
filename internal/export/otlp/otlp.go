// Package otlp exports metrics over OTLP/HTTP or OTLP/gRPC. Metric names
// and labels are metricsd's own; identity travels as OpenTelemetry
// resource attributes so any OTLP backend can group and link sources.
package otlp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"google.golang.org/grpc/credentials"

	"github.com/brian-nunez/metricsd/internal/config"
	"github.com/brian-nunez/metricsd/internal/store"
	"github.com/brian-nunez/metricsd/internal/wire"
)

const scopeName = "github.com/brian-nunez/metricsd"

// Retries inside one Export call stay short; the export runner backs off
// between calls and keeps undelivered data in the store.
var retry = struct{ initial, max, elapsed time.Duration }{time.Second, 5 * time.Second, 20 * time.Second}

type Exporter struct {
	protocol string
	exp      sdkmetric.Exporter
	res      *resource.Resource
	scope    instrumentation.Scope
	attrs    map[string]attribute.Set
}

func New(ctx context.Context, cfg config.OTLPConfig, info wire.Info) (*Exporter, error) {
	tlsCfg, err := tlsConfig(cfg)
	if err != nil {
		return nil, err
	}

	var exp sdkmetric.Exporter
	switch cfg.Protocol {
	case "grpc":
		exp, err = newGRPC(ctx, cfg, tlsCfg)
	default:
		exp, err = newHTTP(ctx, cfg, tlsCfg)
	}
	if err != nil {
		return nil, fmt.Errorf("create otlp %s exporter: %w", cfg.Protocol, err)
	}

	return &Exporter{
		protocol: cfg.Protocol,
		exp:      exp,
		res:      resource.NewWithAttributes(semconv.SchemaURL, resourceAttributes(info)...),
		scope:    instrumentation.Scope{Name: scopeName, Version: info.AgentVersion},
		attrs:    make(map[string]attribute.Set),
	}, nil
}

func (e *Exporter) Name() string { return "otlp-" + e.protocol }

func (e *Exporter) Export(ctx context.Context, cycles []store.Cycle) error {
	points := make(map[string][]metricdata.DataPoint[float64])
	for _, c := range cycles {
		t := time.Unix(c.TS, 0)
		for _, s := range c.Samples {
			points[s.Metric] = append(points[s.Metric], metricdata.DataPoint[float64]{
				Attributes: e.attributes(s.Labels),
				Time:       t,
				Value:      s.Value,
			})
		}
	}

	names := make([]string, 0, len(points))
	for name := range points {
		names = append(names, name)
	}
	sort.Strings(names)

	metrics := make([]metricdata.Metrics, 0, len(names))
	for _, name := range names {
		metrics = append(metrics, metricdata.Metrics{
			Name: name,
			Unit: wire.Unit(name),
			Data: metricdata.Gauge[float64]{DataPoints: points[name]},
		})
	}

	return e.exp.Export(ctx, &metricdata.ResourceMetrics{
		Resource:     e.res,
		ScopeMetrics: []metricdata.ScopeMetrics{{Scope: e.scope, Metrics: metrics}},
	})
}

func (e *Exporter) Shutdown(ctx context.Context) error {
	return e.exp.Shutdown(ctx)
}

// attributes converts a label string to an attribute set. Label strings
// repeat every cycle, so sets are cached; the cache is reset if churning
// labels (containers coming and going) grow it without bound.
func (e *Exporter) attributes(labels string) attribute.Set {
	if set, ok := e.attrs[labels]; ok {
		return set
	}
	if len(e.attrs) > 10000 {
		clear(e.attrs)
	}
	var kvs []attribute.KeyValue
	if labels != "" {
		for _, pair := range strings.Split(labels, ",") {
			if k, v, ok := strings.Cut(pair, "="); ok {
				kvs = append(kvs, attribute.String(k, v))
			}
		}
	}
	set := attribute.NewSet(kvs...)
	e.attrs[labels] = set
	return set
}

func resourceAttributes(info wire.Info) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.ServiceName("metricsd"),
		semconv.ServiceVersion(info.AgentVersion),
		semconv.HostName(info.Hostname),
		semconv.HostArchKey.String(info.Arch),
		semconv.OSTypeKey.String(info.OS),
		attribute.String("metricsd.environment", info.Environment),
		attribute.String("metricsd.id", info.ID),
	}
	if desc := strings.TrimSpace(info.Platform + " " + info.PlatformVersion); desc != "" {
		attrs = append(attrs, semconv.OSDescription(desc))
	}
	if info.Hypervisor != "" {
		attrs = append(attrs, attribute.String("metricsd.hypervisor", info.Hypervisor))
	}
	if info.Container == nil && info.Kubernetes == nil {
		attrs = append(attrs, semconv.HostID(info.ID))
	}
	if c := info.Container; c != nil {
		if c.ID != "" {
			attrs = append(attrs, semconv.ContainerID(c.ID))
		}
		if c.Runtime != "" {
			attrs = append(attrs, semconv.ContainerRuntimeName(c.Runtime))
		}
	}
	if k := info.Kubernetes; k != nil {
		for _, kv := range []struct {
			value string
			attr  func(string) attribute.KeyValue
		}{
			{k.Namespace, semconv.K8SNamespaceName},
			{k.Pod, semconv.K8SPodName},
			{k.PodUID, semconv.K8SPodUID},
			{k.Container, semconv.K8SContainerName},
			{k.Node, semconv.K8SNodeName},
		} {
			if kv.value != "" {
				attrs = append(attrs, kv.attr(kv.value))
			}
		}
	}

	keys := make([]string, 0, len(info.Tags))
	for k := range info.Tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		attrs = append(attrs, attribute.String(k, info.Tags[k]))
	}
	return attrs
}

func newHTTP(ctx context.Context, cfg config.OTLPConfig, tlsCfg *tls.Config) (sdkmetric.Exporter, error) {
	opts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithTimeout(cfg.Timeout.D()),
		otlpmetrichttp.WithRetry(otlpmetrichttp.RetryConfig{
			Enabled:         true,
			InitialInterval: retry.initial,
			MaxInterval:     retry.max,
			MaxElapsedTime:  retry.elapsed,
		}),
	}
	if cfg.Endpoint != "" {
		endpoint, err := httpEndpoint(cfg.Endpoint)
		if err != nil {
			return nil, err
		}
		opts = append(opts, otlpmetrichttp.WithEndpointURL(endpoint))
	}
	if len(cfg.Headers) > 0 {
		opts = append(opts, otlpmetrichttp.WithHeaders(cfg.Headers))
	}
	if cfg.Compression == "gzip" {
		opts = append(opts, otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression))
	} else {
		opts = append(opts, otlpmetrichttp.WithCompression(otlpmetrichttp.NoCompression))
	}
	if tlsCfg != nil {
		opts = append(opts, otlpmetrichttp.WithTLSClientConfig(tlsCfg))
	}
	return otlpmetrichttp.New(ctx, opts...)
}

func newGRPC(ctx context.Context, cfg config.OTLPConfig, tlsCfg *tls.Config) (sdkmetric.Exporter, error) {
	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithTimeout(cfg.Timeout.D()),
		otlpmetricgrpc.WithRetry(otlpmetricgrpc.RetryConfig{
			Enabled:         true,
			InitialInterval: retry.initial,
			MaxInterval:     retry.max,
			MaxElapsedTime:  retry.elapsed,
		}),
	}
	if cfg.Endpoint != "" {
		opts = append(opts, otlpmetricgrpc.WithEndpointURL(cfg.Endpoint))
	}
	if len(cfg.Headers) > 0 {
		opts = append(opts, otlpmetricgrpc.WithHeaders(cfg.Headers))
	}
	if cfg.Compression == "gzip" {
		opts = append(opts, otlpmetricgrpc.WithCompressor("gzip"))
	}
	if tlsCfg != nil {
		opts = append(opts, otlpmetricgrpc.WithTLSCredentials(credentials.NewTLS(tlsCfg)))
	}
	return otlpmetricgrpc.New(ctx, opts...)
}

// httpEndpoint adds the standard /v1/metrics path when the URL has none;
// the SDK otherwise posts to "/".
func httpEndpoint(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/v1/metrics"
	}
	return u.String(), nil
}

// tlsConfig returns nil when no custom certificates are configured, so
// the exporter uses the system trust store.
func tlsConfig(cfg config.OTLPConfig) (*tls.Config, error) {
	if cfg.CAFile == "" && cfg.CertFile == "" {
		return nil, nil
	}
	tc := &tls.Config{MinVersion: tls.VersionTLS12}

	if cfg.CAFile != "" {
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read ca_file: %w", err)
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("ca_file %s contains no PEM certificates", cfg.CAFile)
		}
		tc.RootCAs = pool
	}
	if cfg.CertFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client certificate: %w", err)
		}
		tc.Certificates = []tls.Certificate{cert}
	}
	return tc, nil
}
