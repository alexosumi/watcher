# Watcher Helm Chart

Helm chart for deploying the Watcher Kubernetes operator.

## Installation

```bash
# Install with default values
helm install watcher ./helm/watcher --namespace watcher-system --create-namespace

# Install with custom values
helm install watcher ./helm/watcher \
  --namespace watcher-system \
  --create-namespace \
  --set image.repository=your-registry/watcher \
  --set image.tag=v1.0.0 \
  --set replicaCount=3
```

## Upgrade

```bash
helm upgrade watcher ./helm/watcher --namespace watcher-system
```

## Uninstall

```bash
helm uninstall watcher --namespace watcher-system
```

## Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `2` |
| `image.repository` | Image repository | `watcher` |
| `image.tag` | Image tag | `latest` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `leaderElection.enabled` | Enable leader election | `true` |
| `logLevel` | Log level (debug, info, error) | `info` |
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `128Mi` |
| `resources.requests.cpu` | CPU request | `10m` |
| `resources.requests.memory` | Memory request | `64Mi` |

## Example Custom Values

```yaml
# custom-values.yaml
replicaCount: 3

image:
  repository: myregistry/watcher
  tag: "1.0.0"
  pullPolicy: Always

logLevel: "debug"

resources:
  limits:
    cpu: 1000m
    memory: 256Mi
  requests:
    cpu: 100m
    memory: 128Mi
```

Install with custom values:
```bash
helm install watcher ./helm/watcher -f custom-values.yaml --namespace watcher-system --create-namespace
```
