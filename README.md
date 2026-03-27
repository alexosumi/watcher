# Kubernetes Watcher Operator

A Kubernetes operator that monitors pod memory usage and automatically scales memory resources using in-place pod resize capabilities, and optionally manages Apache Airflow pool slots based on Kubernetes workload metrics.

## Features

### Watcher Controller (Pod Memory Resize)
- **In-Place Pod Resize**: Leverages Kubernetes 1.33+ in-place resize feature for zero-downtime memory scaling
- **Memory Monitoring**: Tracks pod memory usage via Kubernetes metrics server
- **Smart Scaling**: Increases memory by configurable percentage (default 50%) up to 99% of node capacity
- **Watch Mode**: Dry-run mode that logs scaling recommendations without applying changes
- **Configurable Reconcile Interval**: Per-CR `reconcileIntervalSeconds` controls how often pods are checked
- **Node Awareness**: Considers available node memory before scaling

### Pool Controller (Airflow Pool Scaling)
- **Airflow Pool Management**: Automatically adjusts Airflow pool slot counts based on workload metrics
- **Scale Signal Selection**: Choose between `scheduled_slots` or `queued_slots` as the scaling trigger
- **Workload-Aware Guards**: Prevents scaling when worker CPU/Memory exceeds thresholds
- **Running Utilization Gate**: Only increases slots when current pool utilization exceeds 95%
- **Safe Reset**: Resets to default slots when idle, with a safeguard to stay above running slots
- **Watch Mode**: Per-CR `mode: watch` logs scaling recommendations without mutating Airflow
- **Optional**: Pool controller only activates when Airflow credentials are configured

### Shared
- **Single Binary**: Both controllers run in one process under a shared controller-runtime Manager
- **Custom Resource Definitions**: Both `Watcher` and `Pool` CRDs under the `watcher.io/v1` API group
- **Leader Election**: Supports multiple operator instances with leader election for high availability
- **Multi-Architecture**: Supports linux/amd64 and linux/arm64

## Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Watcher CRD   │───▶│                  │───▶│  Metrics Server │
└─────────────────┘    │  Watcher Operator │    └─────────────────┘
                       │  (single binary)  │
┌─────────────────┐    │                  │    ┌─────────────────┐
│    Pool CRD     │───▶│                  │───▶│  Airflow API    │
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
- (Optional) Apache Airflow instance with REST API v1 enabled

### Installation

#### Option 1: Using Helm (Recommended)

```bash
# Install from local chart (Watcher only)
helm install watcher ./helm/watcher

# Install with Airflow Pool controller enabled
helm install watcher ./helm/watcher \
  --set airflow.enabled=true \
  --set airflow.secretName=airflow-credentials
```

#### Option 2: Using kubectl

1. **Install CRDs**:
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

4. **(Optional) Create Pool Resource** (requires Airflow credentials):
```bash
kubectl apply -f examples/pool-etl.yaml
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
  reconcileIntervalSeconds: 60        # Optional: reconcile interval (default: 60, min: 5)
  mode: "patch"                        # Optional: "patch" or "watch" (default: patch)
```

### Pool Resource Spec

```yaml
apiVersion: watcher.io/v1
kind: Pool
metadata:
  name: etl
  namespace: airflow
spec:
  airflowPoolName: etl                 # Required: Airflow pool name
  reconcileIntervalSeconds: 30         # Optional: reconcile interval (default: 30, min: 5)
  scaleSignal: Scheduled               # Optional: Scheduled or Queued (default: Scheduled)
  defaultSlots: 128                    # Required: slot count when scale signal is zero
  increasePercent: 5                   # Required: % to increase slots per reconcile
  maxSlots: 1024                       # Optional: cap for slot increases
  mode: patch                          # Optional: "patch" or "watch" (default: patch)
  workload:                            # Required: workload metrics guard
    namespace: spark-operator
    deploymentName: spark-operator-controller
    metric: CPU                        # CPU (millicores) or Memory (Mi)
    threshold: 500                     # Skip scaling if usage exceeds this
```

### Pool Scaling Logic

1. **Fetch Airflow Pool**: GET pool state from Airflow REST API every `reconcileIntervalSeconds`
2. **Scale Signal Check**: Read `scheduled_slots` or `queued_slots` based on `scaleSignal`
3. **When signal is zero (idle)**: Reset slots to `defaultSlots`, unless `running_slots > defaultSlots` (sets to 110% of running)
4. **When signal > 0 (backlog)**:
   - Check workload metrics: skip if deployment CPU/Memory exceeds `threshold`
   - Check running utilization: skip if `running_slots / slots < 0.95`
   - Increase slots by `increasePercent`, capped by `maxSlots`
5. **Patch Airflow**: Update pool via PATCH API (preserves description and include_deferred)

### Airflow Configuration

The Pool controller requires Airflow credentials via environment variables. When credentials are not set, the Pool controller is silently disabled and only the Watcher controller runs.

| Variable | Description |
|----------|-------------|
| `AIRFLOW_HOST` | Airflow webserver URL (e.g. `https://airflow.example.com`) |
| `AIRFLOW_USERNAME` | HTTP Basic Auth username |
| `AIRFLOW_PASSWORD` | HTTP Basic Auth password |
| `DRY_RUN` | Set to `true` to log actions without mutating Airflow |

For Kubernetes deployments, store credentials in a Secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: airflow-credentials
type: Opaque
stringData:
  AIRFLOW_HOST: "https://airflow.example.com"
  AIRFLOW_USERNAME: "admin"
  AIRFLOW_PASSWORD: "changeme"
```

## Development

### Build & Run Locally

```bash
# Build binary
make build

# Run locally (requires kubeconfig)
make run

# Run with Airflow pool controller (set env vars or use .env file)
AIRFLOW_HOST=https://airflow.example.com AIRFLOW_USERNAME=admin AIRFLOW_PASSWORD=secret make run
```

The operator supports `.env` files via godotenv for local development.

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
- `LOG_LEVEL`: Set logging level (`debug`, `info`, `error`) - default: `info`. Setting `debug` also enables development-mode console logging.
- `POD_NAMESPACE`: Used for leader election lease namespace

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
   - Required permissions: pods, pods/resize, nodes, deployments, pools, watchers

4. **Pod Not Scaling**
   - Check pod has memory requests set (default: 100Mi if not set)
   - Verify node has available memory
   - Ensure pod is in Running state
   - Check operator logs: `kubectl logs -n watcher-system deployment/watcher-controller -f`
   - Try watch mode first: set `mode: "watch"` to see recommendations

5. **Pool Controller Not Starting**
   - Verify `AIRFLOW_HOST`, `AIRFLOW_USERNAME`, and `AIRFLOW_PASSWORD` are set
   - Check logs for "Pool controller disabled" message
   - Use `mode: "watch"` on the Pool CR or `DRY_RUN=true` globally to test without mutating Airflow

6. **Airflow API Errors**
   - Check Pool status conditions: `kubectl get pools -o yaml`
   - Verify Airflow REST API v1 is enabled
   - Confirm credentials and network connectivity

### Logs

```bash
# View operator logs
kubectl logs -n watcher-system deployment/watcher-controller -f

# Check watcher resource status
kubectl get watchers -o yaml

# Check pool resource status
kubectl get pools -o yaml
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
- `pool-etl.yaml` - Airflow ETL pool scaling example
- `pool-submit.yaml` - Airflow submit pool scaling example
