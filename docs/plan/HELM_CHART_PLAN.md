# Helm Chart Plan — amazing-gitlab-exporter

**Version:** 1.0  
**Date:** February 16, 2026  
**Helm Version:** v3 (apiVersion: v2)  
**Status:** Planning Phase

---

## Table of Contents

1. [Overview](#overview)
2. [Design Decisions](#design-decisions)
3. [Chart Structure](#chart-structure)
4. [Values Schema](#values-schema)
5. [Templates Specification](#templates-specification)
6. [OpenShift Compatibility](#openshift-compatibility)
7. [Monitoring Integration](#monitoring-integration)
8. [Security](#security)
9. [Testing Strategy](#testing-strategy)
10. [CI/CD Integration](#cicd-integration)
11. [Task Breakdown](#task-breakdown)

---

## 1. Overview

Add a production-grade Helm chart to the **amazing-gitlab-exporter** project enabling
one-command deployment on **Kubernetes ≥ 1.25** and **OpenShift ≥ 4.12**.

### Key Capabilities

| Capability | Detail |
|---|---|
| **Image** | `ghcr.io/tomer1983/amazing-gitlab-exporter` (multi-arch) |
| **HA mode** | Optional Bitnami Redis sub-chart for multi-replica shared state |
| **Prometheus** | Optional `prometheus-community/prometheus` sub-chart — pre-configured to scrape the exporter |
| **Grafana** | Optional `grafana/grafana` sub-chart — pre-provisioned with Prometheus datasource + all 7 dashboards |
| **Monitoring** | ServiceMonitor, PrometheusRule, Grafana dashboard ConfigMaps (also works with external stacks) |
| **Ingress** | Kubernetes Ingress + OpenShift Route (for webhook endpoint) |
| **Config** | Full exporter config via `values.yaml`, mounted as ConfigMap |
| **Secrets** | GitLab token + webhook secret via Secret or external ref |
| **Security** | Non-root, read-only root FS, SecurityContext, NetworkPolicy |

---

## 2. Design Decisions

### 2.1 Helm v3 / apiVersion v2
- No Tiller dependency; all modern features (library charts, JSON Schema validation).
- `kubeVersion` constraint: `>=1.25.0-0` (covers both K8s and OpenShift 4.12+).

### 2.2 Chart Naming
- Chart name: `amazing-gitlab-exporter`
- Default release name convention: `age` (short alias documented).

### 2.3 Image Policy
- Default tag: `{{ .Chart.AppVersion }}` (tracks chart release).
- SHA digest pinning supported via `image.digest`.
- `image.pullPolicy` defaults to `IfNotPresent`.

### 2.4 Configuration Approach
- Exporter config is rendered into a **ConfigMap** from `.Values.exporterConfig` (structured YAML).
- Sensitive values (GitLab token, Redis password, webhook secret) live in a **Secret** (auto-created or user-supplied via `existingSecret`).
- Environment variable overrides via `.Values.extraEnv` / `.Values.extraEnvFrom`.

### 2.5 Redis Sub-chart
- Dependency: `oci://registry-1.docker.io/bitnamicharts/redis` (latest 19.x).
- Disabled by default (`redis.enabled: false`).
- When enabled, chart auto-wires `AGE_REDIS_URL` into the exporter.
- External Redis supported via `externalRedis.url` (stored in Secret).

### 2.6 Prometheus Sub-chart (Full-Stack Mode)

- Dependency: `prometheus-community/prometheus` (latest 25.x).
- Disabled by default (`prometheus.enabled: false`).
- When enabled, deploys a standalone Prometheus server pre-configured to scrape the exporter.
- Auto-generates a scrape config targeting the exporter Service at `{{ include "amazing-gitlab-exporter.fullname" . }}:8080`.
- Configurable retention (`30d` default, matching docker-compose), storage size, and resource limits.
- Sub-components disabled by default: alertmanager, pushgateway, node-exporter, kube-state-metrics (keeps the footprint minimal — only the Prometheus server is deployed).
- Users who already have a Prometheus instance should use `serviceMonitor.enabled: true` instead.

### 2.7 Grafana Sub-chart (Full-Stack Mode)

- Dependency: `grafana/grafana` (latest 8.x).
- Disabled by default (`grafana.enabled: false`).
- When enabled, deploys Grafana pre-provisioned with:
  - **Datasource**: Auto-configured Prometheus datasource pointing to the co-deployed Prometheus or an external URL.
  - **Dashboards**: All 7 project dashboards (ci-cd-analytics, dora-metrics, exporter-activity, job-performance, merge-request-analytics, pipeline-overview, repository-analytics) provisioned via sidecar.
- Default admin credentials configurable (defaults: `admin/admin`, matching docker-compose).
- Persistent storage optional (`1Gi` default).
- Users who already have Grafana should use `grafanaDashboards.enabled: true` to get dashboard ConfigMaps only.

### 2.8 Full-Stack vs BYO-Stack Decision Matrix

| Scenario | What to enable | Description |
|---|---|---|
| **Full-stack** (like docker-compose) | `prometheus.enabled=true`, `grafana.enabled=true` | Self-contained monitoring stack deployed alongside the exporter |
| **Existing Prometheus Operator** | `serviceMonitor.enabled=true`, `prometheusRule.enabled=true` | Exporter auto-discovered by existing Prometheus via ServiceMonitor CRD |
| **Existing Grafana** | `grafanaDashboards.enabled=true` | Dashboards delivered as ConfigMaps for Grafana sidecar |
| **Minimal** | (none) | Just the exporter; users scrape `/metrics` manually |

### 2.9 OpenShift First-Class Support
- Detect OpenShift via `.Capabilities.APIVersions` and auto-create `Route` when `route.enabled: true`.
- No hard dependency on `restricted-v2` SCC; chart runs under it by default.
- `securityContext.runAsUser` left unset (OpenShift assigns UID from namespace range).
- All templates use `{{- if .Capabilities.APIVersions.Has "route.openshift.io/v1" }}` guards.

---

## 3. Chart Structure

```
charts/amazing-gitlab-exporter/
├── Chart.yaml                    # Chart metadata, dependencies, versions
├── Chart.lock                    # Locked dependency versions
├── CHANGELOG.md                  # Release notes (Keep a Changelog format)
├── values.yaml                   # Default configuration
├── values.schema.json            # JSON Schema for values validation
├── README.md                     # Auto-generated docs (helm-docs)
├── NOTES.txt                     # Post-install usage instructions
├── .helmignore                   # Ignore patterns
├── charts/                       # Bundled sub-charts (redis, prometheus, grafana)
├── crds/                         # (empty - reserved for future CRDs)
├── templates/
│   ├── _helpers.tpl              # Named templates & helper functions
│   ├── deployment.yaml           # Main exporter Deployment
│   ├── service.yaml              # ClusterIP Service (metrics + webhook)
│   ├── configmap.yaml            # Exporter configuration
│   ├── secret.yaml               # GitLab token + webhook secret
│   ├── serviceaccount.yaml       # ServiceAccount + optional annotations
│   ├── hpa.yaml                  # HorizontalPodAutoscaler (opt-in)
│   ├── pdb.yaml                  # PodDisruptionBudget (opt-in)
│   ├── ingress.yaml              # Kubernetes Ingress (opt-in)
│   ├── route.yaml                # OpenShift Route (opt-in)
│   ├── networkpolicy.yaml        # NetworkPolicy (opt-in)
│   ├── servicemonitor.yaml       # Prometheus Operator ServiceMonitor
│   ├── prometheusrule.yaml       # PrometheusRule (alert rules)
│   ├── prometheus-configmap.yaml  # Prometheus scrape config (when sub-chart enabled)
│   ├── grafana-dashboards/       # Grafana dashboard ConfigMaps
│   │   ├── ci-cd-analytics.yaml
│   │   ├── dora-metrics.yaml
│   │   ├── exporter-activity.yaml
│   │   ├── job-performance.yaml
│   │   ├── merge-request-analytics.yaml
│   │   ├── pipeline-overview.yaml
│   │   └── repository-analytics.yaml
│   └── tests/
│       ├── test-connection.yaml  # helm test: HTTP connectivity
│       └── test-metrics.yaml     # helm test: /metrics returns data
└── ci/
    ├── default-values.yaml       # CI test: minimal install
    ├── ha-values.yaml            # CI test: HA with Redis
    ├── full-stack-values.yaml    # CI test: Prometheus + Grafana sub-charts
    ├── openshift-values.yaml     # CI test: OpenShift mode
    └── full-values.yaml          # CI test: all features enabled
```

---

## 4. Values Schema

### 4.1 Top-Level Keys

```yaml
# ─── Image ──────────────────────────────────────────────────────────────────────
image:
  repository: ghcr.io/tomer1983/amazing-gitlab-exporter
  tag: ""                          # Defaults to Chart.AppVersion
  digest: ""                       # Optional SHA digest (overrides tag)
  pullPolicy: IfNotPresent
imagePullSecrets: []

# ─── Replicas & Scaling ────────────────────────────────────────────────────────
replicaCount: 1

autoscaling:
  enabled: false
  minReplicas: 2
  maxReplicas: 5
  targetCPUUtilizationPercentage: 80
  targetMemoryUtilizationPercentage: 80
  behavior: {}                     # Advanced HPA behavior policies

# ─── Service Account ───────────────────────────────────────────────────────────
serviceAccount:
  create: true
  name: ""                         # Auto-generated if empty
  annotations: {}                  # e.g., eks.amazonaws.com/role-arn for IRSA
  automountServiceAccountToken: false

# ─── Pod Configuration ─────────────────────────────────────────────────────────
podAnnotations: {}
podLabels: {}
podSecurityContext:
  runAsNonRoot: true
  fsGroup: 65534
  seccompProfile:
    type: RuntimeDefault

securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  runAsNonRoot: true
  capabilities:
    drop: [ALL]

# ─── Resources ─────────────────────────────────────────────────────────────────
resources:
  requests:
    cpu: 50m
    memory: 64Mi
  limits:
    cpu: 500m
    memory: 256Mi

# ─── Service ───────────────────────────────────────────────────────────────────
service:
  type: ClusterIP
  port: 8080                       # Metrics port
  annotations: {}

# ─── Ingress (Kubernetes) ──────────────────────────────────────────────────────
ingress:
  enabled: false
  className: ""                    # e.g., nginx, traefik
  annotations: {}
  hosts:
    - host: age.example.com
      paths:
        - path: /
          pathType: Prefix
  tls: []

# ─── Route (OpenShift) ─────────────────────────────────────────────────────────
route:
  enabled: false
  host: ""                         # Auto-assigned if empty
  path: /
  tls:
    termination: edge
    insecureEdgeTerminationPolicy: Redirect
  annotations: {}

# ─── Probes ────────────────────────────────────────────────────────────────────
livenessProbe:
  httpGet:
    path: /health
    port: http
  initialDelaySeconds: 5
  periodSeconds: 15
  timeoutSeconds: 5
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /ready
    port: http
  initialDelaySeconds: 5
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3

# ─── Scheduling ────────────────────────────────────────────────────────────────
nodeSelector: {}
tolerations: []
affinity: {}
topologySpreadConstraints: []
priorityClassName: ""

# ─── Disruption Budget ─────────────────────────────────────────────────────────
podDisruptionBudget:
  enabled: false
  minAvailable: 1
  # maxUnavailable: 1

# ─── Network Policy ───────────────────────────────────────────────────────────
networkPolicy:
  enabled: false
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: monitoring
      ports:
        - port: 8080
          protocol: TCP
  egress: []                       # Empty = allow all egress

# ─── Exporter Configuration ───────────────────────────────────────────────────
# Structured YAML rendered into ConfigMap and mounted at /etc/age/config.yml.
# See configs/example.yml for full documentation.
exporterConfig:
  log:
    level: info
    format: json
  server:
    listen_address: ":8080"
    webhook:
      enabled: false
  gitlab:
    url: "https://gitlab.com"
    # token is injected from Secret via env var AGE_GITLAB_TOKEN
    max_requests_per_second: 10
    use_graphql: true
  collectors:
    pipelines:
      enabled: true
      interval_seconds: 30
    jobs:
      enabled: true
      interval_seconds: 30
    merge_requests:
      enabled: true
      interval_seconds: 120
    test_reports:
      enabled: true
      interval_seconds: 60
    dora:
      enabled: true
      interval_seconds: 3600
    repository:
      enabled: true
      interval_seconds: 3600
  projects: []
  wildcards: []

# ─── Secrets ───────────────────────────────────────────────────────────────────
# Option A: Chart-managed Secret
gitlabToken: ""                    # Will be stored in a Secret
webhookSecretToken: ""             # Optional: for webhook validation
# Option B: Reference existing Secret
existingSecret:
  name: ""                         # Name of pre-existing Secret
  gitlabTokenKey: "gitlab-token"   # Key within the Secret
  webhookSecretKey: "webhook-secret"

# ─── Extra Env / Volumes ──────────────────────────────────────────────────────
extraEnv: []
extraEnvFrom: []
extraVolumes: []
extraVolumeMounts: []
extraArgs: []

# ─── Redis Sub-chart ──────────────────────────────────────────────────────────
redis:
  enabled: false
  architecture: standalone         # standalone or replication
  auth:
    enabled: true
    password: ""                   # Auto-generated if empty
  master:
    persistence:
      size: 1Gi

# ─── External Redis ──────────────────────────────────────────────────────────
externalRedis:
  url: ""                          # redis://user:pass@host:6379/0

# ─── Prometheus Sub-chart (Full-Stack Mode) ───────────────────────────────────
# Deploys a standalone Prometheus server pre-configured to scrape the exporter.
# If you already have Prometheus, use serviceMonitor.enabled instead.
prometheus:
  enabled: false
  server:
    retention: "30d"               # Matches docker-compose default
    persistentVolume:
      enabled: true
      size: 10Gi
    resources:
      requests:
        cpu: 100m
        memory: 256Mi
      limits:
        cpu: 1000m
        memory: 2Gi
    extraFlags:
      - web.enable-lifecycle        # Enable /-/reload endpoint
  # Disable unnecessary sub-components for a lightweight deployment
  alertmanager:
    enabled: false
  prometheus-pushgateway:
    enabled: false
  prometheus-node-exporter:
    enabled: false
  kube-state-metrics:
    enabled: false
  # Scrape config auto-generated by the chart (targets the exporter Service)
  serverFiles:
    prometheus.yml:
      scrape_configs: []           # Auto-populated by template helper

# ─── Grafana Sub-chart (Full-Stack Mode) ──────────────────────────────────────
# Deploys Grafana pre-provisioned with Prometheus datasource and all dashboards.
# If you already have Grafana, use grafanaDashboards.enabled instead.
grafana:
  enabled: false
  adminUser: admin
  adminPassword: admin             # Matches docker-compose default
  persistence:
    enabled: true
    size: 1Gi
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 512Mi
  # Sidecar auto-discovers dashboard ConfigMaps with grafana_dashboard label
  sidecar:
    dashboards:
      enabled: true
      label: grafana_dashboard
      labelValue: "1"
      folderAnnotation: grafana_folder
    datasources:
      enabled: true
      label: grafana_datasource
      labelValue: "1"
  # Datasource is auto-provisioned pointing to co-deployed Prometheus or external
  datasources: {}                  # Auto-populated by template helper
  # Additional Grafana config
  grafana.ini:
    users:
      allow_sign_up: false
    auth.anonymous:
      enabled: false

# ─── External Prometheus (for Grafana datasource) ────────────────────────────
# Only used when grafana.enabled=true but prometheus.enabled=false.
externalPrometheus:
  url: ""                          # e.g., http://prometheus.monitoring:9090

# ─── Monitoring (for existing Prometheus Operator stacks) ─────────────────────
serviceMonitor:
  enabled: false
  namespace: ""                    # Deploy into specific namespace
  interval: 30s
  scrapeTimeout: 10s
  labels: {}                       # Extra labels for ServiceMonitor
  relabelings: []
  metricRelabelings: []

prometheusRule:
  enabled: false
  namespace: ""
  labels: {}
  rules: []                        # Override default rules entirely
  # Default rules (applied when rules is empty):
  #   - ExporterDown (critical)
  #   - HighGitLabAPIErrors (warning)
  #   - PipelineFailureRateHigh (warning)
  #   - ScrapeDurationHigh (warning)

grafanaDashboards:
  enabled: false
  labels:
    grafana_dashboard: "1"         # Label for Grafana sidecar auto-discovery
  annotations: {}
  namespace: ""                    # Deploy dashboards to specific namespace
```

### 4.2 JSON Schema (`values.schema.json`)

A JSON Schema file will be provided for `helm lint` and IDE validation, covering:
- Required fields validation
- Type checking for all values
- Enum constraints (e.g., `image.pullPolicy`, `route.tls.termination`)
- Conditional requirements (e.g., `gitlabToken` required when `existingSecret.name` is empty)
- Pattern validation for URLs, resource quantities

---

## 5. Templates Specification

### 5.1 `_helpers.tpl` — Named Templates

| Template | Purpose |
|---|---|
| `amazing-gitlab-exporter.name` | Chart name (truncated to 63 chars) |
| `amazing-gitlab-exporter.fullname` | `release-name-chart-name` (truncated) |
| `amazing-gitlab-exporter.chart` | `chart-version` for labels |
| `amazing-gitlab-exporter.labels` | Standard Kubernetes labels (app.kubernetes.io/*) |
| `amazing-gitlab-exporter.selectorLabels` | Selector subset for Deployment/Service |
| `amazing-gitlab-exporter.serviceAccountName` | Resolved SA name |
| `amazing-gitlab-exporter.redisUrl` | Computed Redis URL (sub-chart or external) |
| `amazing-gitlab-exporter.image` | Full image reference (digest or tag) |
| `amazing-gitlab-exporter.prometheusUrl` | Computed Prometheus URL (sub-chart or external) |
| `amazing-gitlab-exporter.prometheusScrapeConfig` | Auto-generated scrape config for the exporter |

### 5.2 `deployment.yaml`

Key design points:
- **Args**: `["run", "--config", "/etc/age/config.yml"]`
- **Env injection**: `AGE_GITLAB_TOKEN` from Secret, `AGE_REDIS_URL` when Redis enabled, `AGE_WEBHOOK_SECRET_TOKEN` when webhook enabled.
- **Volume mounts**: ConfigMap at `/etc/age/config.yml` (subPath), optional CA cert volume.
- **Security**: Full securityContext from values, read-only root filesystem.
- **Topology**: Supports `topologySpreadConstraints` for HA spread across zones.
- **Init containers**: None required (exporter handles startup checks internally).
- **Checksum annotation**: `checksum/config` and `checksum/secret` to trigger rollout on config change.

### 5.3 `service.yaml`

```yaml
ports:
  - name: http
    port: {{ .Values.service.port }}
    targetPort: http
    protocol: TCP
```

### 5.4 `configmap.yaml`

- Renders `.Values.exporterConfig` as YAML into the ConfigMap data key `config.yml`.
- Uses `toYaml` with proper indentation.
- Strips sensitive fields (token) from the ConfigMap — those are injected via env vars.

### 5.5 `secret.yaml`

- Only created when `existingSecret.name` is empty.
- Contains: `gitlab-token`, `webhook-secret`, `redis-password` (when external Redis used).
- All values base64-encoded via `b64enc`.

### 5.6 `ingress.yaml`

- Standard Kubernetes `networking.k8s.io/v1` Ingress.
- Supports `ingressClassName` field (K8s 1.18+).
- Multi-host and TLS configuration.

### 5.7 `route.yaml` (OpenShift)

- Guarded by `{{- if and .Values.route.enabled (.Capabilities.APIVersions.Has "route.openshift.io/v1") }}`.
- Supports TLS edge/reencrypt/passthrough termination.
- Auto-assigned hostname when `.Values.route.host` is empty.

### 5.8 `networkpolicy.yaml`

- Default allows ingress from monitoring namespace on port 8080.
- Egress rules configurable (GitLab API, Redis, DNS).
- Guarded by `.Values.networkPolicy.enabled`.

### 5.9 `hpa.yaml`

- Uses `autoscaling/v2` API.
- Supports CPU, memory, and custom metrics targets.
- Scaling behavior configuration (stabilization windows, policies).
- Guarded by `.Values.autoscaling.enabled`.

### 5.10 `pdb.yaml`

- Uses `policy/v1` API.
- Supports `minAvailable` or `maxUnavailable`.
- Guarded by `.Values.podDisruptionBudget.enabled`.

### 5.11 `prometheus-configmap.yaml`

- ConfigMap containing a Prometheus scrape configuration that targets the exporter Service.
- Only created when `prometheus.enabled: true`.
- Mounted into the Prometheus sub-chart server via `extraConfigmapMounts`.
- Scrape config:
  ```yaml
  scrape_configs:
    - job_name: "amazing-gitlab-exporter"
      scrape_interval: 30s
      scrape_timeout: 25s
      metrics_path: /metrics
      static_configs:
        - targets: ["{{ include \"amazing-gitlab-exporter.fullname\" . }}:{{ .Values.service.port }}"]
          labels:
            instance: "age"
    - job_name: "prometheus"
      scrape_interval: 60s
      static_configs:
        - targets: ["localhost:9090"]
  ```

### 5.12 Grafana Auto-Provisioning

When `grafana.enabled: true`, the chart automatically:

1. **Datasource**: Creates a ConfigMap labeled `grafana_datasource: "1"` containing the Prometheus datasource config. Points to:
   - Co-deployed Prometheus: `http://{{ .Release.Name }}-prometheus-server:80` (when `prometheus.enabled: true`)
   - External Prometheus: `{{ .Values.externalPrometheus.url }}` (when only Grafana is enabled)

2. **Dashboards**: `grafanaDashboards.enabled` is automatically set to `true` when `grafana.enabled: true`, so all 7 dashboard ConfigMaps are created with proper labels for sidecar discovery.

3. **Dashboard folder**: Dashboards are provisioned into an "Amazing GitLab Exporter" folder via the `grafana_folder` annotation.

---

## 6. OpenShift Compatibility

### 6.1 Security Context Constraints (SCC)

The chart is designed to run under OpenShift's default `restricted-v2` SCC:

| Requirement | Chart Behavior |
|---|---|
| `runAsNonRoot: true` | ✅ Set in `podSecurityContext` |
| No `runAsUser` hardcoded | ✅ Left unset; OpenShift assigns from namespace range |
| `readOnlyRootFilesystem` | ✅ Enabled by default |
| `allowPrivilegeEscalation: false` | ✅ Enabled by default |
| `drop: ALL` capabilities | ✅ Enabled by default |
| `seccompProfile: RuntimeDefault` | ✅ Set in `podSecurityContext` |

### 6.2 Route vs Ingress

- `route.enabled: true` creates an OpenShift Route resource.
- `ingress.enabled: true` creates a standard Kubernetes Ingress.
- Both can coexist but typically only one is used.
- Route template checks for `route.openshift.io/v1` API availability.

### 6.3 Platform Detection Helpers

```yaml
{{- define "amazing-gitlab-exporter.isOpenShift" -}}
{{- if .Capabilities.APIVersions.Has "route.openshift.io/v1" -}}true{{- end -}}
{{- end -}}
```

---

## 7. Monitoring Integration

### 7.1 ServiceMonitor

- Auto-discovers the exporter Service by label selectors.
- Configurable `interval`, `scrapeTimeout`, `relabelings`, `metricRelabelings`.
- Namespace-scoped or cluster-wide (configurable).

### 7.2 PrometheusRule

Default alert rules shipped with the chart:

| Alert | Severity | Condition |
|---|---|---|
| `AgeExporterDown` | critical | `up{job="amazing-gitlab-exporter"} == 0` for 5m |
| `AgeHighGitLabAPIErrors` | warning | Error rate > 5% over 15m |
| `AgePipelineFailureRateHigh` | warning | Pipeline failure rate > 30% over 1h |
| `AgeScrapeDurationHigh` | warning | Scrape duration > 25s for 10m |
| `AgeGitLabRateLimited` | warning | Rate limit hits > 0 for 10m |
| `AgeRedisConnectionFailure` | critical | Redis connection errors > 0 for 5m (HA mode) |

### 7.3 Grafana Dashboard ConfigMaps

- Each dashboard from `grafana/dashboards/*.json` is packaged into a separate ConfigMap.
- Labeled with `grafana_dashboard: "1"` for auto-discovery by the Grafana sidecar.
- Configurable target namespace and extra labels/annotations.
- Dashboard JSON is embedded using `.Files.Get` to avoid template rendering issues.

### 7.4 Full-Stack Mode (Prometheus + Grafana Sub-charts)

When both `prometheus.enabled` and `grafana.enabled` are `true`, the chart deploys a complete self-contained monitoring stack identical to the docker-compose setup:

```
┌─────────────┐     scrape      ┌──────────────────┐     query      ┌─────────┐
│  Prometheus  │ ◄────────────── │ amazing-gitlab-   │ ──────────── ► │ Grafana │
│  (sub-chart) │   :8080/metrics │ exporter          │                │ (sub-ch)│
│  :9090       │                 │ (main deployment) │                │ :3000   │
└──────┬───────┘                 └────────┬──────────┘                └─────────┘
       │                                  │ (optional)
       │                                  ▼
       │                           ┌────────────┐
       │                           │   Redis    │
       │                           │ (sub-chart)│
       │                           └────────────┘
       ▼
  PV (10Gi default)
```

**Component Interaction:**
- Prometheus scrapes the exporter at the `Service` address (not pod IP) for stability.
- Grafana datasource is auto-wired to the Prometheus server URL.
- All 7 dashboards are provisioned automatically.
- Users can access Grafana via Ingress/Route or port-forward.
- Redis is independent and only needed for HA mode.

**Resource Footprint (defaults):**

| Component | CPU Request | Memory Request | Storage |
|---|---|---|---|
| Exporter | 50m | 64Mi | — |
| Prometheus | 100m | 256Mi | 10Gi PV |
| Grafana | 50m | 128Mi | 1Gi PV |
| Redis | 50m | 64Mi | 1Gi PV |
| **Total (full-stack)** | **250m** | **512Mi** | **12Gi** |

---

## 8. Security

### 8.1 Pod Security Standards

The chart enforces **Restricted** Pod Security Standards (PSS) by default:

```yaml
# Pod-level
podSecurityContext:
  runAsNonRoot: true
  fsGroup: 65534
  seccompProfile:
    type: RuntimeDefault

# Container-level
securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  runAsNonRoot: true
  capabilities:
    drop: [ALL]
```

### 8.2 Secret Management

- GitLab token is **never** placed in ConfigMap; always injected via env var from Secret.
- Supports referencing pre-existing Secrets (`existingSecret`).
- Compatible with external secret operators (External Secrets, Sealed Secrets, Vault).

### 8.3 Network Policy

- Default: allow ingress only from Prometheus namespace on metrics port.
- Egress rules can restrict traffic to GitLab API, Redis, and DNS only.

### 8.4 RBAC

- Minimal ServiceAccount with `automountServiceAccountToken: false` by default.
- No ClusterRole/ClusterRoleBinding required (exporter only needs outbound HTTP).

---

## 9. Testing Strategy

### 9.1 Helm Lint & Template Tests

```bash
# Lint with values schema validation
helm lint charts/amazing-gitlab-exporter

# Template rendering (dry-run)
helm template test charts/amazing-gitlab-exporter -f ci/default-values.yaml
helm template test charts/amazing-gitlab-exporter -f ci/ha-values.yaml
helm template test charts/amazing-gitlab-exporter -f ci/openshift-values.yaml
helm template test charts/amazing-gitlab-exporter -f ci/full-values.yaml
```

### 9.2 Helm Unit Tests

Use `helm-unittest` plugin for template assertion tests:

```yaml
# tests/deployment_test.yaml
suite: Deployment
templates:
  - deployment.yaml
tests:
  - it: should set correct image
    set:
      image.tag: "1.2.3"
    asserts:
      - equal:
          path: spec.template.spec.containers[0].image
          value: ghcr.io/tomer1983/amazing-gitlab-exporter:1.2.3

  - it: should inject Redis URL when Redis enabled
    set:
      redis.enabled: true
    asserts:
      - contains:
          path: spec.template.spec.containers[0].env
          content:
            name: AGE_REDIS_URL
```

### 9.3 Helm Integration Tests (`helm test`)

- `test-connection.yaml`: HTTP GET to `/health` endpoint → expect 200.
- `test-metrics.yaml`: HTTP GET to `/metrics` endpoint → expect Prometheus text format.

### 9.4 CI Test Matrix

| Scenario | Values File | Validates |
|---|---|---|
| Minimal | `ci/default-values.yaml` | Basic single-replica deployment |
| HA Mode | `ci/ha-values.yaml` | Redis sub-chart, 3 replicas, PDB |
| Full-Stack | `ci/full-stack-values.yaml` | Prometheus + Grafana sub-charts, dashboards |
| OpenShift | `ci/openshift-values.yaml` | Route, no hardcoded UID, restricted SCC |
| Full | `ci/full-values.yaml` | All features enabled simultaneously |

---

## 10. CI/CD Integration

### 10.1 GitHub Actions Workflow

New workflow `.github/workflows/helm-chart.yml`:

```yaml
name: Helm Chart CI

on:
  push:
    paths: ['charts/**']
  pull_request:
    paths: ['charts/**']

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: azure/setup-helm@v4
      - run: helm lint charts/amazing-gitlab-exporter

  template-test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        values: [default, ha, full-stack, openshift, full]
    steps:
      - uses: actions/checkout@v4
      - uses: azure/setup-helm@v4
      - run: helm template test charts/amazing-gitlab-exporter
               -f charts/amazing-gitlab-exporter/ci/${{ matrix.values }}-values.yaml

  unit-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: azure/setup-helm@v4
      - run: helm plugin install https://github.com/helm-unittest/helm-unittest
      - run: helm unittest charts/amazing-gitlab-exporter

  publish:
    if: startsWith(github.ref, 'refs/tags/chart-')
    needs: [lint, template-test, unit-test]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: azure/setup-helm@v4

      - name: Extract chart version
        id: chart
        run: |
          VERSION=$(grep '^version:' charts/amazing-gitlab-exporter/Chart.yaml | awk '{print $2}')
          echo "version=$VERSION" >> $GITHUB_OUTPUT

      - name: Package & push chart
        run: |
          helm package charts/amazing-gitlab-exporter
          helm push amazing-gitlab-exporter-*.tgz oci://ghcr.io/tomer1983/charts

      - name: Create GitHub Release
        uses: softprops/action-gh-release@v2
        with:
          name: "Helm Chart v${{ steps.chart.outputs.version }}"
          body_path: charts/amazing-gitlab-exporter/CHANGELOG.md
          files: amazing-gitlab-exporter-*.tgz
          tag_name: ${{ github.ref_name }}
          generate_release_notes: true
```

### 10.2 Chart Version Management & Release Notes

The chart version is managed independently from the application version using a dual-version strategy:

| Version Field | Purpose | Example |
|---|---|---|
| `Chart.yaml → version` | Helm chart version (SemVer) | `0.1.0`, `1.0.0`, `1.2.3` |
| `Chart.yaml → appVersion` | Exporter Docker image tag | `v1.0.1`, `v1.1.0` |

**Versioning Rules:**
- Chart `version` follows [SemVer 2.0](https://semver.org/):
  - **MAJOR**: Breaking changes to values schema (renamed/removed keys, changed defaults that break upgrades)
  - **MINOR**: New features (new templates, new values keys, sub-chart upgrades)
  - **PATCH**: Bug fixes, documentation updates, non-breaking default changes
- `appVersion` tracks the exporter release tag and is updated when a new exporter image is published.
- Chart and app versions can be released independently (e.g., chart bugfix without new app release).

**Release Notes:**

A `CHANGELOG.md` file is maintained in the chart directory (`charts/amazing-gitlab-exporter/CHANGELOG.md`) using [Keep a Changelog](https://keepachangelog.com/) format:

```markdown
# Changelog

All notable changes to the amazing-gitlab-exporter Helm chart.

## [Unreleased]

## [1.0.0] - 2026-XX-XX
### Added
- Initial Helm chart release
- Kubernetes and OpenShift support
- Optional Prometheus, Grafana, and Redis sub-charts
- ServiceMonitor, PrometheusRule, Grafana dashboard ConfigMaps
- OpenShift Route support
- NetworkPolicy, PDB, HPA templates
- JSON Schema validation for values
- helm-unittest test suites

### Changed
- N/A

### Fixed
- N/A
```

**GitHub Release Automation:**

The publish workflow creates a GitHub Release with auto-generated release notes when a chart tag is pushed:

```yaml
  publish:
    if: startsWith(github.ref, 'refs/tags/chart-')
    needs: [lint, template-test, unit-test]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: azure/setup-helm@v4

      - name: Extract chart version
        id: chart
        run: |
          VERSION=$(grep '^version:' charts/amazing-gitlab-exporter/Chart.yaml | awk '{print $2}')
          echo "version=$VERSION" >> $GITHUB_OUTPUT

      - name: Package & push chart
        run: |
          helm package charts/amazing-gitlab-exporter
          helm push amazing-gitlab-exporter-*.tgz oci://ghcr.io/tomer1983/charts

      - name: Create GitHub Release
        uses: softprops/action-gh-release@v2
        with:
          name: "Helm Chart v${{ steps.chart.outputs.version }}"
          body_path: charts/amazing-gitlab-exporter/CHANGELOG.md
          files: amazing-gitlab-exporter-*.tgz
          tag_name: ${{ github.ref_name }}
          generate_release_notes: true
```

**Tagging Convention:**

| Tag Pattern | Purpose | Example |
|---|---|---|
| `v*` | Exporter application release | `v1.0.1` |
| `chart-*` | Helm chart release | `chart-0.1.0`, `chart-1.0.0` |

This separation allows publishing chart-only releases without re-releasing the exporter binary.

**ArtifactHub Annotations:**

Chart.yaml includes annotations for ArtifactHub changelogs:

```yaml
annotations:
  artifacthub.io/changes: |
    - kind: added
      description: Initial Helm chart release
    - kind: added
      description: Optional Prometheus and Grafana sub-charts
  artifacthub.io/prerelease: "false"
```

### 10.3 Documentation Generation

- Use `helm-docs` to auto-generate `README.md` from `values.yaml` comments.
- Include install instructions, configuration reference, and upgrade notes.
- CHANGELOG.md maintained manually alongside each chart version bump.

---

## 11. Task Breakdown

### Phase 1: Chart Scaffolding & Core Templates

| # | Task | Description | Status |
|---|---|---|---|
| 1.1 | **Create Chart.yaml** | Chart metadata, apiVersion v2, dependencies (redis sub-chart condition), kubeVersion constraint, annotations for ArtifactHub | ☐ |
| 1.2 | **Create .helmignore** | Ignore ci/, tests/, README template, .git | ☐ |
| 1.3 | **Create _helpers.tpl** | All named templates: name, fullname, chart, labels, selectorLabels, serviceAccountName, image, redisUrl, isOpenShift | ☐ |
| 1.4 | **Create values.yaml** | Full default values as specified in §4 with inline documentation comments | ☐ |
| 1.5 | **Create values.schema.json** | JSON Schema for values validation | ☐ |
| 1.6 | **Create NOTES.txt** | Post-install instructions (URLs, commands, next steps) | ☐ |

### Phase 2: Core Kubernetes Resources

| # | Task | Description | Status |
|---|---|---|---|
| 2.1 | **Create deployment.yaml** | Deployment with env injection, volume mounts, probes, security context, checksum annotations, topology constraints | ☐ |
| 2.2 | **Create service.yaml** | ClusterIP Service exposing metrics port | ☐ |
| 2.3 | **Create configmap.yaml** | Exporter config from values, token-stripped | ☐ |
| 2.4 | **Create secret.yaml** | GitLab token, webhook secret, conditional creation | ☐ |
| 2.5 | **Create serviceaccount.yaml** | Optional SA with annotations | ☐ |

### Phase 3: Optional Resources

| # | Task | Description | Status |
|---|---|---|---|
| 3.1 | **Create ingress.yaml** | Kubernetes Ingress with multi-host, TLS, className | ☐ |
| 3.2 | **Create route.yaml** | OpenShift Route with TLS termination options | ☐ |
| 3.3 | **Create hpa.yaml** | HorizontalPodAutoscaler v2 with behavior config | ☐ |
| 3.4 | **Create pdb.yaml** | PodDisruptionBudget v1 | ☐ |
| 3.5 | **Create networkpolicy.yaml** | NetworkPolicy with configurable ingress/egress | ☐ |

### Phase 4: Monitoring Integration

| # | Task | Description | Status |
|---|---|---|---|
| 4.1 | **Create servicemonitor.yaml** | Prometheus Operator ServiceMonitor | ☐ |
| 4.2 | **Create prometheusrule.yaml** | Default alert rules (6 alerts) | ☐ |
| 4.3 | **Create Grafana dashboard ConfigMaps** | 7 dashboard ConfigMaps using .Files.Get, labeled for sidecar discovery | ☐ |

### Phase 5: Testing

| # | Task | Description | Status |
|---|---|---|---|
| 5.1 | **Create helm test pods** | test-connection.yaml, test-metrics.yaml | ☐ |
| 5.2 | **Create CI value files** | default-values.yaml, ha-values.yaml, openshift-values.yaml, full-values.yaml | ☐ |
| 5.3 | **Create unit tests** | helm-unittest test suites for all templates | ☐ |
| 5.4 | **Run helm lint** | Validate chart structure and schema | ☐ |
| 5.5 | **Run helm template** | Verify rendering with all CI value files | ☐ |

### Phase 6: Documentation & CI

| # | Task | Description | Status |
|---|---|---|---|
| 6.1 | **Create chart README.md** | Install instructions, configuration reference, examples | ☐ |
| 6.2 | **Create GitHub Actions workflow** | helm-chart.yml with lint, template, unit-test, publish jobs | ☐ |

### Phase 7: Sub-chart Integration (Redis, Prometheus, Grafana)

| # | Task | Description | Status |
|---|---|---|---|
| 7.1 | **Add Bitnami Redis dependency** | Chart.yaml dependency with condition `redis.enabled` | ☐ |
| 7.2 | **Wire Redis URL helper** | Auto-compute URL from sub-chart or external config | ☐ |
| 7.3 | **Add Prometheus community dependency** | Chart.yaml dependency with condition `prometheus.enabled`; pin ~25.x | ☐ |
| 7.4 | **Create prometheus-configmap.yaml** | Auto-generated scrape config targeting exporter Service + self-scrape | ☐ |
| 7.5 | **Wire Prometheus URL helper** | Auto-compute Prometheus server URL for Grafana datasource | ☐ |
| 7.6 | **Configure Prometheus server defaults** | Disable alertmanager/pushgateway/node-exporter/kube-state-metrics; set 30d retention | ☐ |
| 7.7 | **Add Grafana community dependency** | Chart.yaml dependency with condition `grafana.enabled`; pin ~8.x | ☐ |
| 7.8 | **Create Grafana datasource ConfigMap** | Auto-provisioned Prometheus datasource (co-deployed or external URL) | ☐ |
| 7.9 | **Wire Grafana dashboard auto-enable** | When `grafana.enabled=true`, ensure `grafanaDashboards.enabled=true` implicitly | ☐ |
| 7.10 | **Configure Grafana defaults** | Sidecar for dashboards + datasources, admin credentials, anonymous auth disabled | ☐ |
| 7.11 | **Create ci/full-stack-values.yaml** | CI values file enabling Prometheus + Grafana + dashboards | ☐ |
| 7.12 | **Test HA deployment** | Validate 3-replica + Redis deployment | ☐ |
| 7.13 | **Test full-stack deployment** | Validate exporter + Prometheus + Grafana end-to-end | ☐ |

### Phase 8: Version Management & Final Documentation

| # | Task | Description | Status |
|---|---|---|---|
| 8.1 | **Create CHANGELOG.md** | Initial changelog in Keep a Changelog format with all v1.0.0 entries | ☐ |
| 8.2 | **Set Chart.yaml versions** | Set `version: 1.0.0`, `appVersion: v1.0.1`; add ArtifactHub changelog annotations | ☐ |
| 8.3 | **Configure chart release tags** | Document `chart-*` tag convention; update CI workflow trigger to `refs/tags/chart-*` | ☐ |
| 8.4 | **Add GitHub Release workflow step** | Extend helm-chart.yml publish job with `softprops/action-gh-release` and release notes | ☐ |
| 8.5 | **Update chart README.md** | Add versioning policy, upgrade guide, and CHANGELOG reference to chart README | ☐ |
| 8.6 | **Update root README.md** | Add Helm chart section: install commands, values reference link, badge for chart version | ☐ |
| 8.7 | **Add ArtifactHub metadata** | Create `artifacthub-repo.yml` at repo root for repository-level discoverability | ☐ |
| 8.8 | **Final review & validation** | Verify all templates render, all tests pass, README links work, CHANGELOG is complete | ☐ |

---

## Appendix A: Quick Start Examples

### Minimal Install (Kubernetes)

```bash
helm install age oci://ghcr.io/tomer1983/charts/amazing-gitlab-exporter \
  --set gitlabToken="glpat-xxxxxxxxxxxxxxxxxxxx" \
  --set exporterConfig.projects[0].name="my-group/my-project"
```

### Full-Stack Install (like docker-compose)

```bash
helm install age oci://ghcr.io/tomer1983/charts/amazing-gitlab-exporter \
  --set gitlabToken="glpat-xxxxxxxxxxxxxxxxxxxx" \
  --set exporterConfig.projects[0].name="my-group/my-project" \
  --set prometheus.enabled=true \
  --set grafana.enabled=true
```

This deploys the exporter + Prometheus (pre-configured to scrape it) + Grafana
(pre-provisioned with Prometheus datasource and all 7 dashboards). Equivalent to
the docker-compose stack.

### Full-Stack with HA

```bash
helm install age oci://ghcr.io/tomer1983/charts/amazing-gitlab-exporter \
  --set gitlabToken="glpat-xxxxxxxxxxxxxxxxxxxx" \
  --set replicaCount=3 \
  --set redis.enabled=true \
  --set prometheus.enabled=true \
  --set grafana.enabled=true \
  --set podDisruptionBudget.enabled=true
```

### BYO Prometheus (existing stack)

```bash
helm install age oci://ghcr.io/tomer1983/charts/amazing-gitlab-exporter \
  --set gitlabToken="glpat-xxxxxxxxxxxxxxxxxxxx" \
  --set serviceMonitor.enabled=true \
  --set prometheusRule.enabled=true \
  --set grafanaDashboards.enabled=true
```

### Grafana Sub-chart with External Prometheus

```bash
helm install age oci://ghcr.io/tomer1983/charts/amazing-gitlab-exporter \
  --set gitlabToken="glpat-xxxxxxxxxxxxxxxxxxxx" \
  --set grafana.enabled=true \
  --set externalPrometheus.url="http://prometheus.monitoring:9090"
```

### OpenShift Install

```bash
helm install age oci://ghcr.io/tomer1983/charts/amazing-gitlab-exporter \
  --set gitlabToken="glpat-xxxxxxxxxxxxxxxxxxxx" \
  --set route.enabled=true \
  --set serviceMonitor.enabled=true
```

### Using Existing Secret

```bash
kubectl create secret generic age-gitlab \
  --from-literal=gitlab-token="glpat-xxxxxxxxxxxxxxxxxxxx"

helm install age oci://ghcr.io/tomer1983/charts/amazing-gitlab-exporter \
  --set existingSecret.name=age-gitlab
```

---

## Appendix B: Compatibility Matrix

| Platform | Version | Tested |
|---|---|---|
| Kubernetes | 1.25 – 1.31 | ☐ |
| OpenShift | 4.12 – 4.17 | ☐ |
| EKS | 1.27 – 1.31 | ☐ |
| GKE | 1.27 – 1.31 | ☐ |
| AKS | 1.27 – 1.31 | ☐ |
| k3s | 1.27+ | ☐ |
| Helm | 3.12+ | ☐ |
