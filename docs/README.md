# metricsd documentation

## Getting started

**New here? Start with [Onboarding](onboarding.md)**: a production config plus install and management steps for VMs, Docker, Podman, and Kubernetes.

1. [Installation](installation.md) – build, install, and start the service (or build the Docker image)
2. [Usage](usage.md) – read metrics with `metricsctl`
3. [Configuration](configuration.md) – tune interval, retention, filters, and tags
4. [Export](export.md) – send metrics to a central server over OTLP

## Environments

- [Containers](containers.md) – Docker, Podman (including rootless), and Kubernetes

## Reference

- [Metrics](metrics.md) – every metric, label, unit, and source, per environment
- [API](api.md) – HTTP endpoints over the Unix socket
- [Configuration](configuration.md) – every config key
- [Examples](../examples/README.md) – annotated configs for common setups

## Operations

- [systemd](systemd.md) – service management, sandboxing, drop-ins, logs
- [Troubleshooting](troubleshooting.md) – common errors and fixes
- [Uninstall](uninstall.md) – remove everything cleanly

## Internals

- [Architecture](architecture.md) – data flow, environment detection, storage schema, disk usage sizing
