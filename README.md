# amazing-gitlab-exporter

[![Build](https://github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/actions/workflows/test.yml/badge.svg)](https://github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/actions/workflows/test.yml)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev)
[![Docker](https://img.shields.io/docker/v/amazing-gitlab-exporter/amazing-gitlab-exporter?logo=docker&label=Docker)](https://hub.docker.com/r/amazing-gitlab-exporter/amazing-gitlab-exporter)

A next-generation **Prometheus exporter for GitLab** (SaaS & self-hosted) that provides comprehensive CI/CD analytics, histogram-based duration metrics, full child/remote pipeline support, and pre-built Grafana dashboards.

---

## Features

| Feature | amazing-gitlab-exporter | gitlab-ci-pipelines-exporter |
|---------|:-----------------------:|:----------------------------:|
| Pipeline metrics | ✅ | ✅ |
| Job metrics | ✅ | ✅ |
| **Histogram durations (P50/P95/P99)** | ✅ | ❌ (gauge only) |
| **Child/remote pipeline tracking** | ✅ | ❌ |
| **Pipeline source tracking** | ✅ (push/MR/schedule/trigger/API) | ❌ |
| **Merge request analytics** | ✅ | ❌ |
| **DORA metrics** (Ultimate) | ✅ | ❌ |
| **Value Stream Analytics** (Premium) | ✅ | ❌ |
| **Code review analytics** | ✅ | ❌ |
| **Repository analytics** | ✅ | ❌ |
| **Contributor analytics** | ✅ | ❌ |
| Tier auto-detection | ✅ | ❌ |
| GraphQL batch queries | ✅ (60-70% fewer API calls) | ❌ (REST only) |
| Pre-built Grafana dashboards | ✅ (6 dashboards) | ✅ (limited) |
| Redis HA mode | ✅ | ✅ |
| GitLab webhooks | ✅ | ✅ |
| Unique metrics | **100+** | ~30 |

### Key Differentiators

- **Histogram-based durations** — native `histogram_quantile()` for pipeline and job P50/P95/P99 percentiles
- **Complete child/remote pipeline visibility** — parent-child relationship tracking with bridge labels
- **All GitLab Analytics sections** — CI/CD, DORA, MR, code review, repository, value stream, contributors
- **Efficient API usage** — hybrid REST + GraphQL approach reduces API calls by 60-70%
- **Automatic tier detection** — gracefully degrades on Free/Premium/Ultimate tiers

---

## Quick Start

### Docker Compose (recommended)

```bash
# Clone the repository
git clone https://github.com/amazing-gitlab-exporter/amazing-gitlab-exporter.git
cd amazing-gitlab-exporter

# Set your GitLab token
export AGE_GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx

# Start all services (exporter + Prometheus + Grafana + Redis)
docker-compose up -d
```

Access:
- **Metrics**: http://localhost:8080/metrics
- **Prometheus**: http://localhost:9090
- **Grafana**: http://localhost:3000 (admin/admin)

### Docker Run

```bash
docker run -d \
  --name amazing-gitlab-exporter \
  -p 8080:8080 \
  -e AGE_GITLAB_URL=https://gitlab.com \
  -e AGE_GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx \
  -v $(pwd)/configs/minimal.yml:/etc/age/config.yml \
  ghcr.io/amazing-gitlab-exporter/amazing-gitlab-exporter:latest \
  run --config /etc/age/config.yml
```

### Binary

Download from [Releases](https://github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/releases):

```bash
# Linux/macOS
./amazing-gitlab-exporter run --config config.yml

# Or configure via environment variables
AGE_GITLAB_URL=https://gitlab.com \
AGE_GITLAB_TOKEN=glpat-xxx \
./amazing-gitlab-exporter run
```

### Kubernetes / Helm Chart

Production-ready Helm chart with full observability stack (Prometheus + Grafana + Redis):

```bash
# Add Helm repository
helm repo add amazing-gitlab-exporter https://tomer1983.github.io/amazing-gitlab-exporter
helm repo update

# Install with minimal configuration
helm install my-exporter amazing-gitlab-exporter/amazing-gitlab-exporter \
  --set secrets.gitlabToken="<your-gitlab-token>" \
  --set exporterConfig.gitlab_url="https://gitlab.example.com" \
  --set exporterConfig.projects[0]="my-group/my-project"

# Or install with full monitoring stack
helm install my-exporter amazing-gitlab-exporter/amazing-gitlab-exporter \
  --set secrets.gitlabToken="<your-gitlab-token>" \
  --set exporterConfig.gitlab_url="https://gitlab.example.com" \
  --set exporterConfig.projects[0]="my-group/my-project" \
  --set prometheus.enabled=true \
  --set grafana.enabled=true \
  --set grafanaDashboards.enabled=true
```

**Features:**
- ✅ Kubernetes 1.25+ and OpenShift 4.12+ support
- 🔐 Security hardened (non-root, read-only rootfs, restricted PSS)
- 📊 Optional Prometheus, Grafana, and Redis sub-charts
- 📈 7 pre-configured Grafana dashboards
- 🚀 Production ready (HPA, PDB, ServiceMonitor, NetworkPolicy)

**Documentation:** [charts/amazing-gitlab-exporter/README.md](charts/amazing-gitlab-exporter/README.md)

---

## Configuration

Configuration supports YAML files, environment variables (`AGE_` prefix), and CLI flags.

**Priority**: CLI flags > Environment variables > Config file > Defaults

### Minimal Config

```yaml
gitlab:
  url: "https://gitlab.com"
  token: ""  # Set via AGE_GITLAB_TOKEN

wildcards:
  - owner:
      name: my-group
      kind: group
      include_subgroups: true
```

### Key Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `AGE_GITLAB_URL` | GitLab instance URL | `https://gitlab.com` |
| `AGE_GITLAB_TOKEN` | Personal Access Token (api + read_repository) | — |
| `AGE_LOG_LEVEL` | Log level (trace/debug/info/warn/error) | `info` |
| `AGE_SERVER_LISTEN_ADDRESS` | Listen address | `:8080` |
| `AGE_REDIS_URL` | Redis URL for HA mode | — |

See [configs/example.yml](configs/example.yml) for the full annotated configuration reference.

---

## Metrics

All metrics use the `age_` prefix. The exporter produces **100+ unique metrics** across these categories:

### Pipeline Metrics (Free Tier)
`age_pipeline_duration_seconds` (histogram), `age_pipeline_queued_duration_seconds` (histogram), `age_pipeline_status`, `age_pipeline_run_count`, `age_pipeline_coverage`, `age_child_pipeline_*`

### Job Metrics (Free Tier)
`age_job_duration_seconds` (histogram), `age_job_queued_duration_seconds` (histogram), `age_job_status`, `age_job_run_count`, `age_job_artifact_size_bytes`

### Merge Request Analytics (Free + Premium)
`age_mr_time_to_merge_seconds` (histogram), `age_mr_throughput_count`, `age_mr_review_cycles_count`, `age_mr_changes_count`, `age_mr_status`

### DORA Metrics (Ultimate)
`age_dora_deployment_frequency`, `age_dora_lead_time_for_changes_seconds`, `age_dora_time_to_restore_service_seconds`, `age_dora_change_failure_rate`

### Repository & Contributor Analytics (Free)
`age_repository_language_percentage`, `age_repository_coverage`, `age_repository_size_bytes`, `age_contributor_commits_count`, `age_contributor_additions`, `age_contributor_deletions`

### Test Reports, Environments, Value Stream, Code Review
See the [full metrics catalog](docs/plan/PROJECT_PLAN.md#prometheus-metrics-catalog) for the complete list.

### Internal Metrics
`age_api_requests_total`, `age_api_request_duration_seconds`, `age_api_rate_limit_remaining`, `age_scrape_duration_seconds`, `age_gitlab_tier`

---

## Grafana Dashboards

Six pre-built dashboards are included in [grafana/dashboards/](grafana/dashboards/) and automatically provisioned with docker-compose.

### Pipeline Overview
<!-- ![Pipeline Overview](docs/screenshots/pipeline-overview.png) -->
Pipeline success rate, status distribution, P50/P95 duration trends, queue time, pipelines by source, child pipeline breakdown, and failed pipeline list.

### Job Performance
<!-- ![Job Performance](docs/screenshots/job-performance.png) -->
Top 20 slowest jobs (P95), failure rate by job, stage duration stacked bars, runner utilization, queue time by stage, artifact sizes, and failure reasons.

### Merge Request Analytics
<!-- ![MR Analytics](docs/screenshots/mr-analytics.png) -->
Time to merge distribution, MR throughput (merged/day), review cycles, size distribution, open MR aging, and velocity trends.

### DORA Metrics
<!-- ![DORA Metrics](docs/screenshots/dora-metrics.png) -->
Four DORA metric gauges (deployment frequency, lead time, MTTR, change failure rate) with trend lines and a monthly comparison table.

### CI/CD Analytics
<!-- ![CI/CD Analytics](docs/screenshots/cicd-analytics.png) -->
Total runs, median duration, success/failure rates, status distribution over time, P50/P95 duration trends, and pipelines per day.

### Repository Analytics
<!-- ![Repository Analytics](docs/screenshots/repository-analytics.png) -->
Language breakdown, coverage trend, commit frequency, repository size, top contributors, and additions/deletions by contributor.

**Import manually**: Each JSON file can be imported directly into Grafana via *Dashboards > Import*. Select your Prometheus datasource when prompted.

---

## Deployment

### Docker

```bash
docker pull ghcr.io/amazing-gitlab-exporter/amazing-gitlab-exporter:latest
```

Multi-arch images are published for `linux/amd64` and `linux/arm64`.

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: amazing-gitlab-exporter
  labels:
    app: amazing-gitlab-exporter
spec:
  replicas: 1
  selector:
    matchLabels:
      app: amazing-gitlab-exporter
  template:
    metadata:
      labels:
        app: amazing-gitlab-exporter
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
        prometheus.io/path: "/metrics"
    spec:
      containers:
        - name: exporter
          image: ghcr.io/amazing-gitlab-exporter/amazing-gitlab-exporter:latest
          args: ["run", "--config", "/etc/age/config.yml"]
          ports:
            - containerPort: 8080
              name: http-metrics
          env:
            - name: AGE_GITLAB_TOKEN
              valueFrom:
                secretKeyRef:
                  name: gitlab-exporter-secret
                  key: token
          volumeMounts:
            - name: config
              mountPath: /etc/age
          livenessProbe:
            httpGet:
              path: /health
              port: http-metrics
            initialDelaySeconds: 10
            periodSeconds: 30
          readinessProbe:
            httpGet:
              path: /ready
              port: http-metrics
            initialDelaySeconds: 5
            periodSeconds: 10
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
            limits:
              cpu: 500m
              memory: 256Mi
      volumes:
        - name: config
          configMap:
            name: amazing-gitlab-exporter-config
---
apiVersion: v1
kind: Service
metadata:
  name: amazing-gitlab-exporter
  labels:
    app: amazing-gitlab-exporter
spec:
  ports:
    - port: 8080
      targetPort: http-metrics
      protocol: TCP
      name: http-metrics
  selector:
    app: amazing-gitlab-exporter
---
apiVersion: v1
kind: Secret
metadata:
  name: gitlab-exporter-secret
type: Opaque
stringData:
  token: "glpat-xxxxxxxxxxxxxxxxxxxx"
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: amazing-gitlab-exporter
  labels:
    app: amazing-gitlab-exporter
spec:
  selector:
    matchLabels:
      app: amazing-gitlab-exporter
  endpoints:
    - port: http-metrics
      interval: 30s
      path: /metrics
```

### HA Mode (Redis)

For multiple replicas, configure Redis to share state:

```yaml
redis:
  url: "redis://redis:6379/0"
```

---

## Development

### Prerequisites

- Go 1.22+
- Docker (optional, for docker-compose)

### Build

```bash
# Build binary
go build -o amazing-gitlab-exporter ./cmd/amazing-gitlab-exporter

# Run
./amazing-gitlab-exporter run --config configs/example.yml
```

### Test

```bash
# Run all tests
go test -race -coverprofile=coverage.txt ./...

# Lint
golangci-lint run
```

### Release

Releases are automated via GitHub Actions and [GoReleaser](.goreleaser.yml):

```bash
git tag v1.0.0
git push origin v1.0.0
```

This triggers:
1. GoReleaser builds binaries for all platforms
2. Docker multi-arch images pushed to ghcr.io and Docker Hub
3. GitHub Release created with changelog and artifacts

### Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Commit changes (use [Conventional Commits](https://www.conventionalcommits.org/))
4. Push and open a Pull Request

---

## License

[Apache License 2.0](LICENSE)

Copyright 2026 amazing-gitlab-exporter contributors.
