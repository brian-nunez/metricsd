# kubernetes-pod

metricsd running inside an application container in a Kubernetes pod. It measures **that container** (its CPU and memory limits, its own IO) and exports to an OpenTelemetry Collector Service in the cluster.

To measure the **node** instead, run metricsd on the node itself; see [kubernetes-node](../kubernetes-node/).

## Settings

| Key | Value | Why |
| --- | ----- | --- |
| `interval` | `15s` | A common cluster monitoring cadence. |
| `retention` | `1d` | Pod storage is ephemeral; the local database is only a buffer for export. |
| `[tags] cluster` | `prod-us-east` | Kubernetes doesn't tell a pod which cluster it's in. |
| `[export.otlp]` | gRPC to the collector Service | Every container reports to one place. |

metricsd detects Kubernetes from `KUBERNETES_SERVICE_HOST` and reports `Environment kubernetes`.

## Identity

| Field | Source |
| ----- | ------ |
| Namespace | Service account namespace file, or `POD_NAMESPACE` |
| Pod | `POD_NAME`, or the hostname |
| Pod UID, container name | Kubelet mount paths |
| Node | `NODE_NAME` |
| ID | `<pod-uid>/<container-name>`, unique per container and stable for the pod's life |

These are exported as `k8s.namespace.name`, `k8s.pod.name`, `k8s.pod.uid`, `k8s.container.name`, and `k8s.node.name` resource attributes. Set the three environment variables with the downward API, as below, so they're always present.

## Deployment

The image is your application image with metricsd copied in (see [docker-container](../docker-container/)).

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-app-metricsd
data:
  config.toml: |
    interval  = "15s"
    retention = "1d"

    [tags]
    cluster = "prod-us-east"

    [export.otlp]
    enabled  = true
    protocol = "grpc"
    endpoint = "http://otel-collector.observability.svc.cluster.local:4317"
    interval = "30s"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 3
  selector:
    matchLabels: { app: my-app }
  template:
    metadata:
      labels: { app: my-app }
    spec:
      containers:
        - name: app
          image: registry.example.com/my-app:1.4.0
          command: ["sh", "-c", "metricsd -config /etc/metricsd/config.toml & exec /app/server"]
          resources:
            requests: { cpu: 250m, memory: 256Mi }
            limits:   { cpu: "1", memory: 1Gi }
          env:
            - name: POD_NAME
              valueFrom: { fieldRef: { fieldPath: metadata.name } }
            - name: POD_NAMESPACE
              valueFrom: { fieldRef: { fieldPath: metadata.namespace } }
            - name: NODE_NAME
              valueFrom: { fieldRef: { fieldPath: spec.nodeName } }
          volumeMounts:
            - name: metricsd-config
              mountPath: /etc/metricsd
              readOnly: true
      volumes:
        - name: metricsd-config
          configMap:
            name: my-app-metricsd
```

With `limits.cpu: "1"` and `limits.memory: 1Gi`, metricsd reports `cpu.cores = 1` and `mem.total_bytes = 1 GiB`. Without limits it reports the node's capacity as the ceiling.

### Endpoint from the environment

To keep the endpoint out of the ConfigMap, delete the `endpoint` line and set the standard variable instead:

```yaml
            - name: OTEL_EXPORTER_OTLP_ENDPOINT
              value: http://otel-collector.observability.svc.cluster.local:4317
```

### Read-only root filesystem

With `securityContext.readOnlyRootFilesystem: true`, give metricsd a writable directory:

```yaml
          volumeMounts:
            - { name: metricsd-data, mountPath: /var/lib/metricsd }   # running as root
            - { name: metricsd-tmp,  mountPath: /tmp }                # non-root: database and socket
      volumes:
        - { name: metricsd-data, emptyDir: {} }
        - { name: metricsd-tmp,  emptyDir: {} }
```

## Check it

```bash
kubectl exec deploy/my-app -c app -- metricsctl info
kubectl exec deploy/my-app -c app -- metricsctl now
```

## Scope

Each container running metricsd reports itself. A sidecar container running metricsd measures only the sidecar, not the app next to it, so add metricsd to the containers you want measured.

More: [docs/containers.md](../../docs/containers.md), [docs/export.md](../../docs/export.md).
