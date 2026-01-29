# Kubernetes Watcher Operator

A Kubernetes operator that monitors pod memory usage and automatically scales memory resources based on configurable thresholds.

## Features

- **Custom Resource Definition (CRD)**: Define monitoring configurations via Watcher resources
- **Memory Monitoring**: Tracks pod memory usage via Kubernetes metrics server
- **Smart Scaling**: Increases memory by 50% or up to 99% of node capacity
- **Leader Election**: Supports multiple operator instances with leader election
- **Namespace & Label Filtering**: Monitor specific pods based on namespace and labels
- **Node Awareness**: Considers available node memory before scaling

## Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Watcher CRD   │───▶│  Watcher Operator │───▶│  Metrics Server │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                                │
                                ▼
                       ┌─────────────────┐
                       │   Target Pods   │
                       └─────────────────┘
```

## Quick Start

### Prerequisites

- Kubernetes cluster with metrics server installed
- kubectl configured
- Docker (for building images)

### Installation

1. **Install CRD**:
```bash
make install-crd
```

2. **Deploy Operator**:
```bash
make deploy
```

3. **Create Watcher Resource**:
```bash
kubectl apply -f examples/watcher-example.yaml
```

## Configuration

### Watcher Resource Spec

```yaml
apiVersion: watcher.io/v1
kind: Watcher
metadata:
  name: my-watcher
spec:
  namespace: "target-namespace"        # Required: namespace to monitor
  labelSelector:                       # Required: pod label selector
    app: "my-app"
    tier: "frontend"
  memoryThreshold: 80                  # Optional: threshold % (default: 80)
  scaleUpPercentage: 50               # Optional: scale increase % (default: 50)
```

### Parameters

- **namespace**: Target namespace containing pods to monitor
- **labelSelector**: Key-value pairs to filter pods
- **memoryThreshold**: Memory usage percentage (1-100) that triggers scaling
- **scaleUpPercentage**: Percentage increase for memory scaling (1-200)

## Scaling Logic

1. **Monitor**: Check pod memory usage every 2 minutes
2. **Threshold Check**: Compare usage against configured threshold
3. **Calculate**: Determine new memory request:
   - Increase by configured percentage (default 50%)
   - Respect node capacity (max 99% of node memory)
4. **Update**: Modify pod memory requests and limits

## Development

### Build & Run Locally

```bash
# Build binary
make build

# Run locally (requires kubeconfig)
make run
```

### Build Docker Image

```bash
make docker-build IMG=your-registry/watcher:tag
make docker-push IMG=your-registry/watcher:tag
```

### Testing

```bash
make test
make fmt
make vet
```

## Deployment Options

### Single Instance
```yaml
spec:
  replicas: 1
```

### High Availability (with Leader Election)
```yaml
spec:
  replicas: 2  # or more
```

Leader election is automatically enabled when `--leader-elect=true` flag is set.

## Monitoring

The operator exposes metrics on port 8080:
- `/metrics` - Prometheus metrics
- `/healthz` - Health check
- `/readyz` - Readiness check

## Security

- Runs as non-root user
- Read-only root filesystem
- Minimal RBAC permissions
- Security context configured

## Troubleshooting

### Common Issues

1. **Metrics Server Not Available**
   - Ensure metrics server is installed: `kubectl get deployment metrics-server -n kube-system`

2. **Permission Denied**
   - Verify RBAC configuration: `kubectl get clusterrolebinding watcher-controller-rolebinding`

3. **Pod Not Scaling**
   - Check pod has memory requests set
   - Verify node has available memory
   - Check operator logs: `kubectl logs -n watcher-system deployment/watcher-controller`

### Logs

```bash
# View operator logs
kubectl logs -n watcher-system deployment/watcher-controller -f

# Check watcher resource status
kubectl get watchers -o yaml
```

## Cleanup

```bash
make undeploy
```