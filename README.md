# Kubernetes Watcher Operator

A Kubernetes operator that monitors pod memory usage and automatically scales memory resources using in-place pod resize capabilities.

## Features

- **In-Place Pod Resize**: Leverages Kubernetes 1.33+ in-place resize feature for zero-downtime memory scaling
- **Custom Resource Definition (CRD)**: Define monitoring configurations via Watcher resources
- **Memory Monitoring**: Tracks pod memory usage via Kubernetes metrics server
- **Smart Scaling**: Increases memory by configurable percentage (default 50%) up to 99% of node capacity
- **Watch Mode**: Dry-run mode that logs scaling recommendations without applying changes
- **Leader Election**: Supports multiple operator instances with leader election for high availability
- **Namespace & Label Filtering**: Monitor specific pods based on namespace and labels
- **Node Awareness**: Considers available node memory before scaling
- **Multi-Architecture**: Supports linux/amd64 and linux/arm64

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

- **Kubernetes 1.33 or greater** (required for in-place pod resize)
- Metrics server installed and running
- kubectl configured
- Docker (for building images)

### Installation

#### Option 1: Using Helm (Recommended)

```bash
# Install from local chart (easiest method)
helm install watcher ./helm/watcher

# Or install from OCI registry (requires authentication)
# 1. Create GitHub Personal Access Token with read:packages scope
# 2. Login to GHCR:
echo $GITHUB_TOKEN | helm registry login ghcr.io -u YOUR_GITHUB_USERNAME --password-stdin

# 3. Install chart:
helm install watcher oci://ghcr.io/YOUR_GITHUB_USERNAME/charts/watcher --version 1.0.0
```

#### Option 2: Using kubectl

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
  mode: "patch"                        # Optional: "patch" or "watch" (default: patch)
```

### Parameters

- **namespace** (required): Target namespace containing pods to monitor
- **labelSelector** (required): Key-value pairs to filter pods
- **memoryThreshold** (optional): Memory usage percentage (1-100) that triggers scaling (default: 80)
- **scaleUpPercentage** (optional): Percentage increase for memory scaling (default: 50)
- **mode** (optional): Operation mode
  - `patch` (default): Apply memory changes using in-place resize
  - `watch`: Log recommendations without applying changes (dry-run)

## Scaling Logic

1. **Monitor**: Check pod memory usage every 60 seconds
2. **Threshold Check**: Compare usage against configured threshold (default 80%)
3. **Calculate**: Determine new memory request:
   - Increase by configured percentage (default 50%)
   - Respect node capacity (max 99% of node memory)
   - Consider current node usage and available memory
4. **Update**: Apply changes using Kubernetes in-place pod resize
   - Updates both requests and limits to the same value
   - No pod restart required
   - 300ms delay between processing pods to avoid API throttling

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
# Build for local platform
make docker-build IMG=your-registry/watcher:tag
make docker-push IMG=your-registry/watcher:tag

# Multi-architecture build (requires buildx)
docker buildx build --platform linux/amd64,linux/arm64 -t your-registry/watcher:tag --push .
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

Leader election is enabled by default in the deployment with `--leader-elect=true` flag.

### Configuration Options

Environment variables:
- `LOG_LEVEL`: Set logging level (`debug`, `info`, `error`) - default: `info`

Command-line flags:
- `--metrics-bind-address`: Metrics endpoint address (default: `:9090`)
- `--health-probe-bind-address`: Health probe address (default: `:8081`)
- `--leader-elect`: Enable leader election (default: `false`)
- `--leader-election-id`: Leader election lock name (default: `watcher-controller`)

## Monitoring

The operator exposes the following endpoints:
- `:9090/metrics` - Prometheus metrics
- `:8081/healthz` - Liveness probe
- `:8081/readyz` - Readiness probe

## Security

- Runs as non-root user (UID 65532)
- Read-only root filesystem
- Minimal RBAC permissions
- Security context configured with seccomp profile
- All capabilities dropped

## Troubleshooting

### Common Issues

1. **Kubernetes Version Too Old**
   - Error: "Watcher operator requires Kubernetes 1.33 or greater"
   - Solution: Upgrade your cluster to Kubernetes 1.33+
   - The operator checks version on startup and via ValidatingAdmissionPolicy

2. **Metrics Server Not Available**
   - Ensure metrics server is installed: `kubectl get deployment metrics-server -n kube-system`
   - Pods without metrics are silently skipped

3. **Permission Denied**
   - Verify RBAC configuration: `kubectl get clusterrolebinding watcher-controller-rolebinding`
   - Required permissions: pods (get/list/watch/patch), pods/resize (get/patch), nodes (get/list/watch)

4. **Pod Not Scaling**
   - Check pod has memory requests set (default: 100Mi if not set)
   - Verify node has available memory
   - Ensure pod is in Running state
   - Check operator logs: `kubectl logs -n watcher-system deployment/watcher-controller -f`
   - Try watch mode first: set `mode: "watch"` to see recommendations

5. **In-Place Resize Failing**
   - Verify Kubernetes version is 1.33+
   - Check if InPlacePodVerticalScaling feature gate is enabled
   - Review pod events: `kubectl describe pod <pod-name>`

### Logs

```bash
# View operator logs
kubectl logs -n watcher-system deployment/watcher-controller -f

# Check watcher resource status
kubectl get watchers -o yaml
```

## CI/CD

The project includes GitHub Actions workflow for automated releases:
- Builds multi-architecture Docker images (amd64/arm64)
- Publishes to GitHub Container Registry (ghcr.io)
- Packages and publishes Helm chart
- Triggered on version tags (`v*`) or manual workflow dispatch

## Cleanup

```bash
# Using kubectl
make undeploy

# Using Helm
helm uninstall watcher
```

## Examples

See the [examples/](examples/) directory for sample configurations:
- `watcher-example.yaml` - Production examples with patch mode
- `watcher-watch-mode.yaml` - Watch mode example (dry-run)
