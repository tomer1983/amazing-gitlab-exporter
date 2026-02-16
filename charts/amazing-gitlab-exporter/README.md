# Amazing GitLab Exporter Helm Chart

A production-ready Helm chart for deploying the Amazing GitLab Exporter — a Prometheus exporter for GitLab CI/CD analytics, DORA metrics, and repository insights.

[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/amazing-gitlab-exporter)](https://artifacthub.io/packages/search?repo=amazing-gitlab-exporter)
[![Helm: v3](https://img.shields.io/badge/Helm-v3-informational?logo=helm)](https://helm.sh/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

## Features

- ✅ **Multi-Platform Support**: Kubernetes ≥1.25 and OpenShift ≥4.12
- 🔐 **Security Hardened**: Restricted Pod Security Standards, non-root, read-only rootfs, seccomp profiles
- 📊 **Full Observability Stack**: Optional Prometheus, Grafana, and Redis sub-charts
- 📈 **Pre-built Dashboards**: 7 Grafana dashboards for CI/CD analytics, DORA metrics, job performance
- 🚀 **Production Ready**: HPA, PDB, NetworkPolicy, ServiceMonitor, PrometheusRule
- 🔄 **High Availability**: Multi-replica support with Redis-backed state persistence
- 🛣️ **OpenShift Native**: Route support with TLS termination, SCC compatibility

## Prerequisites

- Kubernetes 1.25+ or OpenShift 4.12+
- Helm 3.10+
- GitLab API token with `read_api` and `read_repository` scopes

## Quick Start

### Install with Minimal Configuration

```bash
helm repo add amazing-gitlab-exporter https://tomer1983.github.io/amazing-gitlab-exporter
helm repo update

helm install my-exporter amazing-gitlab-exporter/amazing-gitlab-exporter \
  --set secrets.gitlabToken="<your-gitlab-token>" \
  --set exporterConfig.gitlab_url="https://gitlab.example.com" \
  --set exporterConfig.projects[0]="my-group/my-project"
```

### Install with Full Monitoring Stack

```bash
helm install my-exporter amazing-gitlab-exporter/amazing-gitlab-exporter \
  --set secrets.gitlabToken="<your-gitlab-token>" \
  --set exporterConfig.gitlab_url="https://gitlab.example.com" \
  --set exporterConfig.projects[0]="my-group/my-project" \
  --set prometheus.enabled=true \
  --set grafana.enabled=true \
  --set grafanaDashboards.enabled=true
```

### Access Grafana Dashboards

```bash
# Get Grafana admin password
kubectl get secret my-exporter-grafana -o jsonpath="{.data.admin-password}" | base64 -d

# Port-forward to Grafana
kubectl port-forward svc/my-exporter-grafana 3000:80

# Open http://localhost:3000 (username: admin)
```

## Configuration

### Essential Configuration

| Parameter | Description | Default |
|---|---|---|
| `secrets.gitlabToken` | GitLab API token (required) | `""` |
| `exporterConfig.gitlab_url` | GitLab instance URL | `"https://gitlab.com"` |
| `exporterConfig.projects` | List of projects to monitor | `[]` |
| `exporterConfig.collection_interval` | Scrape interval in seconds | `300` |

### Image Configuration

| Parameter | Description | Default |
|---|---|---|
| `image.repository` | Container image repository | `ghcr.io/tomer1983/amazing-gitlab-exporter` |
| `image.tag` | Image tag (overrides appVersion) | `""` |
| `image.digest` | Image digest (takes precedence over tag) | `""` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |

### Deployment Configuration

| Parameter | Description | Default |
|---|---|---|
| `replicaCount` | Number of replicas | `1` |
| `autoscaling.enabled` | Enable HPA | `false` |
| `autoscaling.minReplicas` | Minimum replicas | `1` |
| `autoscaling.maxReplicas` | Maximum replicas | `3` |
| `podDisruptionBudget.enabled` | Enable PDB | `false` |

### Networking

| Parameter | Description | Default |
|---|---|---|
| `service.type` | Service type | `ClusterIP` |
| `service.port` | Service port | `8080` |
| `ingress.enabled` | Enable Ingress | `false` |
| `ingress.className` | Ingress class name | `""` |
| `route.enabled` | Enable OpenShift Route | `false` |

### Sub-Charts

| Parameter | Description | Default |
|---|---|---|
| `redis.enabled` | Deploy Redis for HA state | `false` |
| `prometheus.enabled` | Deploy Prometheus | `false` |
| `grafana.enabled` | Deploy Grafana | `false` |

### Monitoring Integration

| Parameter | Description | Default |
|---|---|---|
| `serviceMonitor.enabled` | Create ServiceMonitor for Prometheus Operator | `false` |
| `prometheusRule.enabled` | Create PrometheusRule with alerts | `false` |
| `grafanaDashboards.enabled` | Create Grafana dashboard ConfigMaps | `false` |

### Security

| Parameter | Description | Default |
|---|---|---|
| `podSecurityContext.runAsNonRoot` | Run as non-root user | `true` |
| `securityContext.readOnlyRootFilesystem` | Read-only root filesystem | `true` |
| `securityContext.allowPrivilegeEscalation` | Allow privilege escalation | `false` |
| `networkPolicy.enabled` | Enable NetworkPolicy | `false` |

## Usage Examples

### High Availability Deployment

```yaml
# ha-values.yaml
replicaCount: 3

redis:
  enabled: true
  architecture: standalone

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 5
  targetCPUUtilizationPercentage: 75

podDisruptionBudget:
  enabled: true
  minAvailable: 1

resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 256Mi
```

```bash
helm install my-exporter amazing-gitlab-exporter/amazing-gitlab-exporter \
  -f ha-values.yaml \
  --set secrets.gitlabToken="<token>"
```

### OpenShift Deployment

```yaml
# openshift-values.yaml
route:
  enabled: true
  host: exporter.apps.example.com
  tls:
    termination: edge
    insecureEdgeTerminationPolicy: Redirect

# OpenShift assigns UIDs dynamically
podSecurityContext:
  runAsNonRoot: true

exporterConfig:
  gitlab_url: "https://gitlab.example.com"
  projects:
    - "devops/infrastructure"
    - "backend/api"
```

```bash
oc new-project gitlab-metrics
helm install my-exporter amazing-gitlab-exporter/amazing-gitlab-exporter \
  -f openshift-values.yaml \
  --set secrets.gitlabToken="<token>"
```

### Existing Prometheus Operator Stack

```yaml
# prometheus-operator-values.yaml
redis:
  enabled: true

serviceMonitor:
  enabled: true
  namespace: monitoring
  interval: 30s
  labels:
    release: prometheus-operator

prometheusRule:
  enabled: true
  namespace: monitoring
  labels:
    release: prometheus-operator

grafanaDashboards:
  enabled: true
  namespace: monitoring
  labels:
    grafana_dashboard: "1"
```

## Dashboards

The chart includes 7 production-ready Grafana dashboards:

1. **Pipeline Overview** — Pipeline success rates, duration trends, queue times
2. **CI/CD Analytics** — Job distribution, runner utilization, cost per pipeline
3. **DORA Metrics** — Deployment frequency, lead time, MTTR, change failure rate
4. **Merge Request Analytics** — Review time, approval metrics, merge trends
5. **Job Performance** — Job duration, failure patterns, retry analysis
6. **Repository Analytics** — Commit activity, contributor metrics, code churn
7. **Exporter Activity** — API rate limits, scrape duration, error rates

Enable dashboards with:

```yaml
grafana:
  enabled: true
  sidecar:
    dashboards:
      enabled: true

grafanaDashboards:
  enabled: true
```

## Alerts

Default PrometheusRule alerts:

- **ExporterDown** (critical) — Exporter pod unavailable for 5+ minutes
- **HighGitLabAPIErrors** (warning) — API error rate >0.1/s for 10 minutes
- **PipelineFailureRateHigh** (warning) — Pipeline failure rate >30% for 30 minutes
- **ScrapeDurationHigh** (warning) — Scrape duration >30s for 5 minutes
- **RateLimitNearExhaustion** (warning) — Remaining rate limit <100
- **HighMemoryUsage** (warning) — Memory usage >90% of limit for 10 minutes

## Upgrading

### From 0.x to 1.x

Version 1.0.0 is the initial stable release. No migration required.

### Chart Upgrades

```bash
# Update repository
helm repo update

# Check for new versions
helm search repo amazing-gitlab-exporter

# Upgrade with existing values
helm upgrade my-exporter amazing-gitlab-exporter/amazing-gitlab-exporter \
  --reuse-values

# Or specify new values
helm upgrade my-exporter amazing-gitlab-exporter/amazing-gitlab-exporter \
  -f my-values.yaml
```

## Uninstalling

```bash
# Uninstall the release
helm uninstall my-exporter

# If using sub-charts with persistent volumes
kubectl delete pvc -l app.kubernetes.io/instance=my-exporter
```

## Troubleshooting

### Check Exporter Logs

```bash
kubectl logs -l app.kubernetes.io/name=amazing-gitlab-exporter -f
```

### Verify Configuration

```bash
# Port-forward to exporter
kubectl port-forward svc/my-exporter-amazing-gitlab-exporter 8080:8080

# Check config endpoint
curl http://localhost:8080/config

# Check metrics
curl http://localhost:8080/metrics
```

### Common Issues

**Chart.yaml file is missing**
- Issue: Windows file encoding with negation patterns in `.helmignore`
- Fix: Upgrade to chart version 1.0.1+ which removes problematic patterns

**GitLab API 401**
- Check token validity: `curl -H "PRIVATE-TOKEN: <token>" https://gitlab.example.com/api/v4/user`
- Verify secret creation: `kubectl get secret my-exporter-amazing-gitlab-exporter -o yaml`

**Rate Limit Errors**
- Increase `collection_interval` to reduce API calls
- Use a dedicated service account with higher rate limits

**Metrics Not Scraped**
- Verify ServiceMonitor labels match Prometheus `serviceMonitorSelector`
- Check Prometheus operator logs

## Development

### Testing Chart Locally

```bash
# Lint the chart
helm lint charts/amazing-gitlab-exporter

# Template rendering
helm template test charts/amazing-gitlab-exporter -f ci/default-values.yaml

# Install with debug
helm install test charts/amazing-gitlab-exporter \
  -f ci/default-values.yaml \
  --dry-run --debug
```

### Running Unit Tests

```bash
# Install helm-unittest plugin
helm plugin install https://github.com/helm-unittest/helm-unittest

# Run tests
helm unittest charts/amazing-gitlab-exporter
```

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](../../CONTRIBUTING.md) for guidelines.

## License

Apache License 2.0 — See [LICENSE](../../LICENSE) for details.

## Links

- [GitHub Repository](https://github.com/tomer1983/amazing-gitlab-exporter)
- [Docker Images](https://github.com/tomer1983/amazing-gitlab-exporter/pkgs/container/amazing-gitlab-exporter)
- [Documentation](https://github.com/tomer1983/amazing-gitlab-exporter/tree/main/docs)
- [Issue Tracker](https://github.com/tomer1983/amazing-gitlab-exporter/issues)

## Support

- 💬 [Discussions](https://github.com/tomer1983/amazing-gitlab-exporter/discussions)
- 🐛 [Issues](https://github.com/tomer1983/amazing-gitlab-exporter/issues)
- 📧 [Contact Maintainers](https://github.com/tomer1983)
