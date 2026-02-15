# amazing-gitlab-exporter - Project Plan

**Version:** 1.0  
**Date:** February 15, 2026  
**Status:** Planning Phase

---

## Executive Summary

**amazing-gitlab-exporter** is a next-generation Prometheus exporter for GitLab (SaaS & self-hosted) designed to surpass the existing `gitlab-ci-pipelines-exporter` by providing:

- **Complete GitLab Analytics coverage**: All metrics from GitLab's Analyze section (CI/CD Analytics, DORA metrics, Code Review Analytics, Repository Analytics, Value Stream Analytics)
- **Histogram-based duration metrics**: Native P50/P95/P99 percentile support for pipelines and jobs
- **Full child/remote pipeline support**: Proper tracking of triggered and parent-child pipeline relationships
- **Efficient API usage**: Hybrid REST + GraphQL approach reducing API calls by 60-70%
- **Production-ready**: Docker/Kubernetes support, HA mode with Redis, GitHub Actions CI/CD
- **Pre-built Grafana dashboards**: 6 comprehensive dashboards for complete observability

---

## Goals & Success Criteria

### Primary Goals

1. **Comprehensive Metrics**: Export all available GitLab analytics metrics (Free/Premium/Ultimate tiers)
2. **Performance**: Reduce API calls by 60-70% vs existing exporters through GraphQL batching and incremental fetching
3. **Remote Pipeline Support**: Full visibility into child/triggered pipelines with parent-child relationship tracking
4. **Better Metrics**: Replace gauge-only duration metrics with histograms for native percentile calculations
5. **Production Ready**: Docker images, Kubernetes support, HA mode, comprehensive monitoring

### Success Metrics

- ✅ Exports 100+ unique Prometheus metrics vs ~30 in existing exporter
- ✅ Supports all GitLab tiers (Free, Premium, Ultimate) with auto-detection
- ✅ Grafana dashboards provide complete CI/CD visibility
- ✅ Docker image < 20MB, multi-arch (amd64/arm64)
- ✅ Automated testing and releases via GitHub Actions

---

## Architecture Overview

### Technology Stack

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| **Language** | Go 1.22+ | Best for Prometheus exporters, excellent concurrency, small binaries |
| **GitLab API** | REST v4 + GraphQL | Hybrid approach for efficiency |
| **Metrics** | Prometheus client_golang | Industry standard, histogram support |
| **Config** | YAML + env vars + CLI flags | Flexible, familiar pattern |
| **Storage** | In-memory + Redis (HA) | Simple by default, scalable when needed |
| **Logging** | logrus | Structured logging with JSON support |
| **CLI** | urfave/cli/v3 | Mature, feature-rich |
| **Deployment** | Docker + Kubernetes | Standard container deployment |

### Core Dependencies

```go
// go.mod (key dependencies)
require (
    gitlab.com/gitlab-org/api/client-go v1.34.0+      // GitLab REST API
    github.com/hasura/go-graphql-client v0.10.0+      // GitLab GraphQL
    github.com/prometheus/client_golang v1.19.0+      // Prometheus metrics
    github.com/redis/go-redis/v9 v9.17.3+             // Redis for HA
    github.com/urfave/cli/v3 v3.6.2+                  // CLI framework
    github.com/sirupsen/logrus v1.9.4+                // Logging
    gopkg.in/yaml.v3 v3.0.1+                          // YAML parsing
    github.com/go-playground/validator/v10 v10.19.0+  // Validation
    github.com/heptiolabs/healthcheck v0.0.0+         // Health checks
    golang.org/x/time v0.5.0+                         // Rate limiting
)
```

### Project Structure

```
amazing-gitlab-exporter/
├── cmd/
│   └── amazing-gitlab-exporter/
│       └── main.go                    # CLI entrypoint
├── internal/
│   ├── config/
│   │   ├── config.go                  # Configuration struct & parsing
│   │   ├── defaults.go                # Default values
│   │   └── validation.go              # Config validation
│   ├── gitlab/
│   │   ├── client.go                  # Unified GitLab client
│   │   ├── rest.go                    # REST API v4 methods
│   │   ├── graphql.go                 # GraphQL queries
│   │   ├── ratelimiter.go             # Rate limiting
│   │   └── tier_detector.go           # GitLab tier detection
│   ├── collector/
│   │   ├── registry.go                # Metric registry
│   │   ├── pipelines.go               # Pipeline metrics
│   │   ├── jobs.go                    # Job metrics
│   │   ├── merge_requests.go          # MR analytics
│   │   ├── environments.go            # Environment/deployment metrics
│   │   ├── test_reports.go            # Test suite metrics
│   │   ├── dora.go                    # DORA metrics (Ultimate)
│   │   ├── value_stream.go            # Value Stream analytics (Premium)
│   │   ├── code_review.go             # Code review analytics
│   │   ├── repository.go              # Repository analytics
│   │   └── contributors.go            # Contributor metrics
│   ├── scheduler/
│   │   ├── scheduler.go               # Task scheduler
│   │   ├── task.go                    # Task definitions
│   │   └── queue.go                   # Task queue (memory/Redis)
│   ├── store/
│   │   ├── interface.go               # Storage interface
│   │   ├── memory.go                  # In-memory store
│   │   └── redis.go                   # Redis store
│   ├── server/
│   │   ├── server.go                  # HTTP server
│   │   └── webhook.go                 # Webhook handler
│   └── exporter/
│       └── exporter.go                # Main orchestrator
├── grafana/
│   └── dashboards/
│       ├── pipeline-overview.json
│       ├── job-performance.json
│       ├── merge-request-analytics.json
│       ├── dora-metrics.json
│       ├── ci-cd-analytics.json
│       └── repository-analytics.json
├── configs/
│   ├── example.yml                     # Full config example
│   └── minimal.yml                     # Minimal config
├── .github/
│   └── workflows/
│       ├── test.yml                    # PR & main branch tests
│       └── release.yml                 # Release automation
├── Dockerfile                          # Multi-stage Docker build
├── docker-compose.yml                  # Local dev environment
├── README.md
├── LICENSE                             # Apache-2.0
└── go.mod
```

---

## Key Features & Differentiators

### 1. Histogram-Based Duration Metrics

**Problem with existing exporter**: Pipeline and job durations are exported as gauges (single values), making it impossible to calculate percentiles (P50/P95/P99) in Prometheus/Grafana.

**Our solution**: Export duration metrics as histograms with configurable buckets:

```yaml
collectors:
  pipelines:
    histogram_buckets: [5, 10, 30, 60, 120, 300, 600, 1800, 3600]  # seconds
  jobs:
    histogram_buckets: [5, 10, 30, 60, 120, 300, 600, 1800]
```

This enables native Prometheus queries like:
```promql
histogram_quantile(0.95, rate(age_pipeline_duration_seconds_bucket[5m]))
```

### 2. Full Child/Remote Pipeline Support

**Problem**: Existing exporter treats child pipelines as independent, losing parent-child relationships.

**Our solution**: 
- Discover child/downstream pipelines via bridge/trigger jobs (`GET /pipelines/:id/bridges`)
- Emit separate `age_child_pipeline_*` metrics with parent relationship labels
- Track cross-project triggered pipelines
- Labels: `parent_project`, `parent_ref`, `bridge_name`

**Example metric**:
```
age_child_pipeline_duration_seconds{
  project="team/backend",
  ref="main",
  parent_project="team/parent",
  parent_ref="main",
  bridge_name="trigger-backend"
} 125.4
```

### 3. Efficient API Usage: REST + GraphQL Hybrid

**GraphQL for batch operations** (reduces API calls by 60-70%):
- Fetch multiple projects with single query
- Batch pipeline/MR/job listing with nested fields
- Reduces pagination overhead

**REST for specialized endpoints**:
- DORA metrics API
- Test reports
- Pipeline variables
- Value Stream Analytics API

**Incremental fetching**: Track `updated_after` timestamps per project to only fetch changed data.

### 4. GitLab Tier Auto-Detection

On startup, probe GitLab instance to detect tier:
- Attempt DORA metrics endpoint → If 403/404, disable DORA collector
- Attempt Value Stream Analytics → If 403/404, disable VSA collector
- Check for Premium/Ultimate features
- Emit `age_gitlab_tier` metric (0=Free, 1=Premium, 2=Ultimate)

Graceful degradation: exporter works on any tier, exports maximum available metrics.

### 5. Complete Metrics Coverage

**Existing exporter**: ~30 metrics (pipelines, jobs, environments, test reports)

**amazing-gitlab-exporter**: 100+ metrics across:
- Pipelines (with source tracking: push/MR/schedule/trigger)
- Jobs (with runner type, stage, failure reasons)
- Child/remote pipelines
- Merge Request analytics (time to merge, review time, cycles)
- Code Review analytics (reviewer workload, turnaround)
- DORA metrics (deployment frequency, lead time, MTTR, change failure rate)
- Value Stream Analytics (stage durations, cycle time)
- Repository analytics (languages, coverage, commit frequency)
- Contributor analytics (commit activity, additions/deletions)
- Environment/deployment tracking
- Test reports (suite/case level)

---

## Prometheus Metrics Catalog

All metrics prefixed with `age_` (amazing-gitlab-exporter).

### Pipeline Metrics (Free Tier)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `age_pipeline_duration_seconds` | Histogram | project, ref, kind, source, status | Pipeline execution duration |
| `age_pipeline_queued_duration_seconds` | Histogram | project, ref, kind, source | Time in queue before execution |
| `age_pipeline_status` | Gauge | project, ref, kind, source, status | Pipeline status (0 or 1) |
| `age_pipeline_run_count` | Counter | project, ref, kind, source | Total pipeline runs |
| `age_pipeline_coverage` | Gauge | project, ref, kind | Code coverage percentage |
| `age_pipeline_id` | Gauge | project, ref, kind | Current pipeline ID |
| `age_pipeline_created_timestamp` | Gauge | project, ref, kind | Pipeline creation timestamp |

**Key improvement**: `source` label distinguishes:
- `push` - Direct push to branch
- `merge_request_event` - MR pipeline
- `schedule` - Scheduled pipeline
- `api` - Triggered via API
- `trigger` - Cross-project trigger
- `parent_pipeline` - Child pipeline
- `web` - Manual run from UI

### Child/Remote Pipeline Metrics (Free Tier)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `age_child_pipeline_duration_seconds` | Histogram | project, ref, parent_project, parent_ref, bridge_name | Child pipeline duration |
| `age_child_pipeline_status` | Gauge | project, ref, parent_project, parent_ref, bridge_name, status | Child pipeline status |
| `age_child_pipeline_run_count` | Counter | project, ref, parent_project, parent_ref, bridge_name | Child pipeline executions |
| `age_child_pipeline_queued_duration_seconds` | Histogram | project, ref, parent_project, parent_ref, bridge_name | Child pipeline queue time |

### Job Metrics (Free Tier)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `age_job_duration_seconds` | Histogram | project, ref, stage, job_name, runner_type, status | Job execution duration |
| `age_job_queued_duration_seconds` | Histogram | project, ref, stage, job_name | Job queue time |
| `age_job_status` | Gauge | project, ref, stage, job_name, status, failure_reason | Job status |
| `age_job_run_count` | Counter | project, ref, stage, job_name | Job executions |
| `age_job_artifact_size_bytes` | Gauge | project, ref, stage, job_name | Artifact size |

**runner_type values**: `instance`, `group`, `project`, `unknown`

### Test Report Metrics (Free Tier)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `age_test_report_total_time_seconds` | Gauge | project, ref | Total test execution time |
| `age_test_report_total_count` | Gauge | project, ref | Total test count |
| `age_test_report_success_count` | Gauge | project, ref | Successful tests |
| `age_test_report_failed_count` | Gauge | project, ref | Failed tests |
| `age_test_report_skipped_count` | Gauge | project, ref | Skipped tests |
| `age_test_report_error_count` | Gauge | project, ref | Tests with errors |
| `age_test_suite_duration_seconds` | Gauge | project, ref, suite_name | Test suite duration |
| `age_test_suite_count` | Gauge | project, ref, suite_name | Tests in suite |
| `age_test_case_duration_seconds` | Histogram | project, ref, suite, case_name | Individual test duration |
| `age_test_case_status` | Gauge | project, ref, suite, case_name, status | Test case status |

### Environment/Deployment Metrics (Free Tier)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `age_environment_deployment_duration_seconds` | Histogram | project, environment | Deployment duration |
| `age_environment_deployment_status` | Gauge | project, environment, status | Deployment status |
| `age_environment_deployment_count` | Counter | project, environment | Total deployments |
| `age_environment_behind_commits` | Gauge | project, environment | Commits behind target |
| `age_environment_behind_duration_seconds` | Gauge | project, environment | Time since last deployment |

### Merge Request Analytics (Free + Premium)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `age_mr_time_to_merge_seconds` | Histogram | project, target_branch | Time from creation to merge |
| `age_mr_time_to_first_review_seconds` | Histogram | project, target_branch | Time to first review action |
| `age_mr_review_cycles_count` | Histogram | project, target_branch | Number of review cycles |
| `age_mr_changes_count` | Histogram | project, target_branch | Lines changed |
| `age_mr_status` | Gauge | project, target_branch, state | MR state (opened/merged/closed) |
| `age_mr_throughput_count` | Counter | project, target_branch | MRs merged |
| `age_mr_notes_count` | Histogram | project, target_branch | Comment count |
| `age_mr_open_duration_seconds` | Histogram | project, target_branch | Time MR stayed open |

### Code Review Analytics (Premium)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `age_review_turnaround_seconds` | Histogram | project, reviewer | Time to provide review |
| `age_review_approval_count` | Counter | project, reviewer | Reviews approved |
| `age_review_pending_count` | Gauge | project | MRs awaiting review |
| `age_review_requested_count` | Counter | project, reviewer | Review requests received |

### DORA Metrics (Ultimate Tier)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `age_dora_deployment_frequency` | Gauge | project, environment_tier | Deployments per day |
| `age_dora_lead_time_for_changes_seconds` | Gauge | project, environment_tier | Commit to deploy time |
| `age_dora_time_to_restore_service_seconds` | Gauge | project, environment_tier | Mean time to recovery |
| `age_dora_change_failure_rate` | Gauge | project, environment_tier | Failed deployment rate |

**environment_tier values**: `production`, `staging`, `testing`, `development`, `other`

### Value Stream Analytics (Premium Tier)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `age_value_stream_stage_duration_seconds` | Gauge | project, stage_name | Time in VSA stage |
| `age_value_stream_cycle_time_seconds` | Gauge | project | Total cycle time |
| `age_value_stream_lead_time_seconds` | Gauge | project | Total lead time |

**stage_name values**: `issue`, `plan`, `code`, `test`, `review`, `staging`, `production`

### Repository Analytics (Free Tier)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `age_repository_language_percentage` | Gauge | project, language | Language breakdown |
| `age_repository_commit_count` | Counter | project, ref | Total commits |
| `age_repository_size_bytes` | Gauge | project | Repository size |
| `age_repository_coverage` | Gauge | project | Test coverage percentage |

### Contributor Analytics (Free Tier)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `age_contributor_commits_count` | Counter | project, author | Commits by author |
| `age_contributor_additions` | Counter | project, author | Lines added |
| `age_contributor_deletions` | Counter | project, author | Lines deleted |

### Internal/Operational Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `age_api_requests_total` | Counter | method, endpoint, status_code | GitLab API calls |
| `age_api_request_duration_seconds` | Histogram | method, endpoint | API latency |
| `age_api_rate_limit_remaining` | Gauge | - | Remaining rate limit |
| `age_api_rate_limit_reset_timestamp` | Gauge | - | Rate limit reset time |
| `age_projects_tracked` | Gauge | - | Projects being monitored |
| `age_scrape_duration_seconds` | Histogram | collector_type | Collector execution time |
| `age_scrape_errors_total` | Counter | collector_type, error_type | Collection errors |
| `age_gitlab_tier` | Gauge | - | Detected tier (0/1/2) |
| `age_collector_enabled` | Gauge | collector_type | Collector status |

---

## Configuration

### Full Configuration Schema

```yaml
# Logging configuration
log:
  level: info                    # trace, debug, info, warn, error, fatal, panic
  format: text                   # text, json

# HTTP server configuration
server:
  listen_address: ":8080"        # Address for metrics/health endpoints
  enable_pprof: false            # Enable pprof profiling endpoints
  webhook:
    enabled: false               # Enable GitLab webhook receiver
    secret_token: ""             # Optional webhook validation token

# Redis for HA mode (leave empty for single-instance)
redis:
  url: ""                        # redis://user:password@host:port/db
  pool_size: 10
  min_idle_conns: 1

# GitLab connection
gitlab:
  url: "https://gitlab.com"      # GitLab instance URL
  token: ""                      # PAT with api + read_repository scopes
  enable_tls_verify: true        # Verify TLS certificates
  ca_cert_path: ""               # Custom CA certificate path
  max_requests_per_second: 10    # Sustained rate limit
  burst_requests_per_second: 20  # Burst allowance
  use_graphql: true              # Enable GraphQL for batch queries
  graphql_page_size: 100         # Items per GraphQL page
  rest_page_size: 100            # Items per REST page

# Collector configuration
collectors:
  # Pipeline metrics
  pipelines:
    enabled: true
    interval_seconds: 30
    include_child_pipelines: true
    histogram_buckets: [5, 10, 30, 60, 120, 300, 600, 1800, 3600]
    max_pipelines_per_ref: 10   # Keep last N pipelines per ref

  # Job metrics
  jobs:
    enabled: true
    interval_seconds: 30
    histogram_buckets: [5, 10, 30, 60, 120, 300, 600, 1800]
    include_runner_details: true
    
  # Merge request analytics
  merge_requests:
    enabled: true
    interval_seconds: 120
    histogram_buckets: [3600, 7200, 14400, 28800, 86400, 172800, 604800]
    
  # Environment/deployment metrics
  environments:
    enabled: false
    interval_seconds: 300
    exclude_stopped: true
    
  # Test report metrics
  test_reports:
    enabled: false
    interval_seconds: 60
    include_test_cases: false    # Individual test case metrics
    
  # DORA metrics (Ultimate tier)
  dora:
    enabled: true                # Auto-disabled if not Ultimate
    interval_seconds: 3600
    environment_tiers:
      - production
      - staging
      
  # Value Stream Analytics (Premium tier)
  value_stream:
    enabled: true                # Auto-disabled if not Premium
    interval_seconds: 3600
    
  # Code review analytics
  code_review:
    enabled: true
    interval_seconds: 300
    
  # Repository analytics
  repository:
    enabled: true
    interval_seconds: 3600
    
  # Contributor analytics
  contributors:
    enabled: false
    interval_seconds: 3600

# Default settings for all projects (can override per-project)
project_defaults:
  output_sparse_status_metrics: true  # Only emit current status, not all statuses
  
  # Ref (branch/tag/MR) configuration
  refs:
    branches:
      enabled: true
      regexp: "^(main|master|develop|release/.*)$"
      most_recent: 0             # 0 = all matching branches
      max_age_days: 0            # 0 = no age limit
      exclude_deleted: true
    tags:
      enabled: true
      regexp: "^v.*"
      most_recent: 10            # Keep last 10 tags
      max_age_days: 90
      exclude_deleted: true
    merge_requests:
      enabled: true
      states: [opened, merged]   # opened, closed, merged, all
      most_recent: 20
      max_age_days: 30

# Explicit project list
projects:
  - name: group/project-a
    # All project_defaults can be overridden here
  - name: group/project-b
    refs:
      branches:
        regexp: "^main$"

# Wildcard project discovery
wildcards:
  - owner:
      name: my-group
      kind: group                # group or user
      include_subgroups: true
    search: ""                   # Optional search term
    archived: false              # Include archived projects
    # All project_defaults can be overridden here
```

### Environment Variable Mapping

All config values can be set via environment variables with `AGE_` prefix:

```bash
AGE_LOG_LEVEL=debug
AGE_GITLAB_URL=https://gitlab.example.com
AGE_GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
AGE_REDIS_URL=redis://localhost:6379/0
AGE_SERVER_LISTEN_ADDRESS=:9090
AGE_COLLECTORS_PIPELINES_ENABLED=true
AGE_COLLECTORS_DORA_ENABLED=false
```

Nested values use underscores: `AGE_COLLECTORS_PIPELINES_INTERVAL_SECONDS=60`

### CLI Flags

```bash
amazing-gitlab-exporter run \
  --config /etc/age/config.yml \
  --gitlab-url https://gitlab.example.com \
  --gitlab-token glpat-xxx \
  --log-level debug \
  --server-listen-address :9090
```

**Priority**: CLI flags > Environment variables > Config file > Defaults

---

## GitLab API Strategy

### 1. REST + GraphQL Hybrid Approach

**GraphQL Usage** (reduces API calls by 60-70%):

```graphql
# Single query to fetch 100 projects with latest pipeline info
query {
  projects(first: 100, after: $cursor) {
    nodes {
      id
      fullPath
      pipelines(first: 10, ref: $ref) {
        nodes {
          id
          status
          duration
          queuedDuration
          createdAt
          finishedAt
          source
          ref
        }
      }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
```

**Advantages**:
- Batch fetch projects + pipelines in single request
- Reduce pagination overhead
- Request only needed fields
- Consistent nested data fetching

**REST Usage** (for endpoints not in GraphQL):
- DORA metrics: `GET /api/v4/projects/:id/dora/metrics`
- Test reports: `GET /api/v4/projects/:id/pipelines/:pid/test_report`
- Pipeline variables: `GET /api/v4/projects/:id/pipelines/:pid/variables`
- Value Stream Analytics: `GET /api/v4/analytics/value_stream_analytics/*`
- Bridge jobs (child pipelines): `GET /api/v4/projects/:id/pipelines/:pid/bridges`

### 2. Incremental Fetching

Track `last_updated_at` timestamp per project and collector type:

```go
// Only fetch pipelines updated since last check
GET /api/v4/projects/:id/pipelines?updated_after=2026-02-15T10:00:00Z

// Only fetch MRs updated since last check
GET /api/v4/projects/:id/merge_requests?updated_after=2026-02-15T10:00:00Z
```

**Impact**: After initial sync, subsequent polls fetch only changed data (typically 5-10% of full dataset).

### 3. Rate Limiting Strategy

**Local token bucket**:
- Configurable: `max_requests_per_second` (sustained) + `burst_requests_per_second`
- Uses `golang.org/x/time/rate` package

**Header-aware backoff**:
```go
// Check response headers
RateLimit-Remaining: 1500
RateLimit-Reset: 1708000000

if remaining < 100 {
    sleep until reset
}
```

**Distributed rate limiting (HA mode)**:
- Use Redis-based rate limiter
- Share quota across all exporter replicas
- Prevents "thundering herd" when multiple instances run

### 4. GitLab Tier Detection

On startup:
1. Call `GET /api/v4/version` to validate connection
2. Attempt `GET /api/v4/projects/:id/dora/metrics` → If 403/404, mark Ultimate features unavailable
3. Attempt `GET /api/v4/projects/:id/analytics/merge_request_analytics` → If 403/404, mark Premium features unavailable
4. Store detected capabilities in `gitlab.DetectedFeatures`
5. Auto-disable tier-gated collectors
6. Emit `age_gitlab_tier` metric

**Graceful degradation**: Exporter works on any tier, maximizing available metrics.

---

## Child/Remote Pipeline Handling

### Discovery Process

1. **Fetch parent pipeline**: `GET /api/v4/projects/:id/pipelines/:pid`
2. **List bridge/trigger jobs**: `GET /api/v4/projects/:id/pipelines/:pid/bridges`
3. **For each bridge with downstream_pipeline**:
   ```json
   {
     "id": 12345,
     "name": "trigger-backend",
     "downstream_pipeline": {
       "id": 67890,
       "project_id": 999,
       "ref": "main",
       "status": "success"
     }
   }
   ```
4. **Fetch child pipeline**: `GET /api/v4/projects/999/pipelines/67890`
5. **Recurse**: Check child pipeline for further downstream pipelines

### Metric Emission

**Parent pipeline metrics** (standard):
```prometheus
age_pipeline_duration_seconds_bucket{
  project="team/api",
  ref="main",
  kind="branch",
  source="push",
  status="success",
  le="120"
} 1
```

**Child pipeline metrics** (with parent context):
```prometheus
age_child_pipeline_duration_seconds_bucket{
  project="team/backend",
  ref="main",
  parent_project="team/api",
  parent_ref="main",
  bridge_name="trigger-backend",
  le="60"
} 1
```

### Cross-Project Handling

For cross-project triggered pipelines:
- Exporter must have API access to all involved projects
- Use wildcard discovery to automatically include downstream projects
- Config example:
  ```yaml
  wildcards:
    - owner:
        name: team
        kind: group
        include_subgroups: true  # Includes all team/* projects
  ```

### Grafana Querying

**Total pipeline time including children**:
```promql
histogram_quantile(0.95,
  sum(rate(age_pipeline_duration_seconds_bucket[5m])) by (le, project, ref)
  +
  sum(rate(age_child_pipeline_duration_seconds_bucket[5m])) by (le, parent_project, parent_ref)
)
```

**Child pipeline failure impact**:
```promql
age_pipeline_status{status="failed"} * on(project, ref) group_left
count(age_child_pipeline_status{status="failed"}) by (parent_project, parent_ref)
```

---

## Grafana Dashboards

Six pre-built dashboards included in `grafana/dashboards/`:

### 1. Pipeline Overview (`pipeline-overview.json`)

**Panels**:
- Success rate % over time (stacked area chart)
- Pipeline count by status (bar chart)
- P50/P95 duration trends (line chart)
- Queue time vs execution time (dual axis)
- Pipeline count by source (push/MR/schedule/trigger)
- Child pipeline breakdown (table)
- Recent failed pipelines (table with links to GitLab)

**Variables**: `project`, `ref`, `time_range`

### 2. Job Performance (`job-performance.json`)

**Panels**:
- Job duration heatmap (stage × job)
- P95 duration by job (bar chart, top 20 slowest)
- Job failure rate % (table with sparklines)
- Stage-level aggregated durations (stacked bar)
- Runner utilization (pie chart by runner_type)
- Queue time by stage (box plot)
- Artifact size trends (area chart)
- Jobs with failure reasons (table)

**Variables**: `project`, `ref`, `stage`, `job_name`

### 3. Merge Request Analytics (`merge-request-analytics.json`)

**Panels**:
- Time to merge distribution (histogram)
- Time to first review distribution (histogram)
- MR throughput (MRs merged per day)
- Review cycles per MR (bar chart)
- MR size distribution (lines changed)
- Open MRs aging report (table)
- MR velocity by target branch (line chart)
- Top reviewers by throughput (bar chart)

**Variables**: `project`, `target_branch`

**Tier**: Free + Premium features

### 4. DORA Metrics (`dora-metrics.json`)

**Panels**:
- Deployment frequency (deployments/day, gauge + trend)
- Lead time for changes (median in hours, gauge + trend)
- Time to restore service (MTTR in hours, gauge + trend)
- Change failure rate (%, gauge + trend)
- DORA score badge (calculated from all 4 metrics)
- Monthly comparison table
- Deployment frequency by environment tier (stacked bar)

**Variables**: `project`, `environment_tier`, `time_range`

**Tier**: Ultimate only

### 5. CI/CD Analytics (`ci-cd-analytics.json`)

Mirrors GitLab's CI/CD Analytics page:

**Panels**:
- Total pipeline runs (stat panel)
- Median duration (stat panel)
- Success rate % (stat panel)
- Failure rate % (stat panel)
- Pipeline status distribution over time (stacked area)
- Duration P50/P95 trends (line chart)
- Pipelines per day (bar chart)

**Variables**: `project`, `ref`

**Tier**: Free

### 6. Repository & Contributors (`repository-analytics.json`)

**Panels**:
- Language breakdown (pie chart)
- Test coverage trend (line chart)
- Commit frequency (bar chart, commits per week)
- Repository size trend (area chart)
- Top contributors by commits (table)
- Lines added/deleted by contributor (stacked bar)
- Commit activity heatmap (day of week × hour)

**Variables**: `project`

**Tier**: Free

### Dashboard Provisioning

To auto-import dashboards into Grafana:

**docker-compose.yml**:
```yaml
grafana:
  image: grafana/grafana:latest
  volumes:
    - ./grafana/dashboards:/etc/grafana/provisioning/dashboards
    - ./grafana/datasources.yml:/etc/grafana/provisioning/datasources/datasources.yml
```

**grafana/datasources.yml**:
```yaml
apiVersion: 1
datasources:
  - name: Prometheus
    type: prometheus
    url: http://prometheus:9090
    isDefault: true
```

---

## Deployment

### Docker

**Dockerfile** (multi-stage):
```dockerfile
# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o amazing-gitlab-exporter ./cmd/amazing-gitlab-exporter

# Runtime stage
FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /build/amazing-gitlab-exporter /amazing-gitlab-exporter
ENTRYPOINT ["/amazing-gitlab-exporter"]
CMD ["run"]
```

**Images published to**:
- `ghcr.io/<owner>/amazing-gitlab-exporter:latest`
- `ghcr.io/<owner>/amazing-gitlab-exporter:v1.0.0`
- `docker.io/<owner>/amazing-gitlab-exporter:latest`

**Multi-arch**: `linux/amd64`, `linux/arm64`

### Kubernetes

**Deployment**:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: amazing-gitlab-exporter
spec:
  replicas: 1  # Set to 2+ with Redis for HA
  selector:
    matchLabels:
      app: amazing-gitlab-exporter
  template:
    metadata:
      labels:
        app: amazing-gitlab-exporter
    spec:
      containers:
      - name: exporter
        image: ghcr.io/<owner>/amazing-gitlab-exporter:latest
        ports:
        - containerPort: 8080
          name: metrics
        env:
        - name: AGE_GITLAB_URL
          value: "https://gitlab.com"
        - name: AGE_GITLAB_TOKEN
          valueFrom:
            secretKeyRef:
              name: gitlab-token
              key: token
        - name: AGE_REDIS_URL
          value: "redis://redis:6379/0"
        volumeMounts:
        - name: config
          mountPath: /etc/age
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 10
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
      volumes:
      - name: config
        configMap:
          name: age-config
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
    name: metrics
  selector:
    app: amazing-gitlab-exporter
---
apiVersion: v1
kind: ServiceMonitor
metadata:
  name: amazing-gitlab-exporter
spec:
  selector:
    matchLabels:
      app: amazing-gitlab-exporter
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
```

**ConfigMap**:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: age-config
data:
  config.yml: |
    log:
      level: info
    collectors:
      pipelines:
        enabled: true
        interval_seconds: 30
    wildcards:
      - owner:
          name: my-group
          kind: group
```

**Secret**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: gitlab-token
type: Opaque
stringData:
  token: glpat-xxxxxxxxxxxxxxxxxxxx
```

### Docker Compose (local dev)

**docker-compose.yml**:
```yaml
version: '3.8'

services:
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis-data:/data

  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheus-data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    volumes:
      - ./grafana/dashboards:/etc/grafana/provisioning/dashboards
      - ./grafana/datasources.yml:/etc/grafana/provisioning/datasources/datasources.yml
      - grafana-data:/var/lib/grafana
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
      - GF_USERS_ALLOW_SIGN_UP=false

  exporter:
    build: .
    ports:
      - "8080:8080"
    environment:
      - AGE_GITLAB_URL=https://gitlab.com
      - AGE_GITLAB_TOKEN=${GITLAB_TOKEN}
      - AGE_REDIS_URL=redis://redis:6379/0
      - AGE_LOG_LEVEL=debug
    volumes:
      - ./configs/example.yml:/etc/age/config.yml
    depends_on:
      - redis
      - prometheus

volumes:
  redis-data:
  prometheus-data:
  grafana-data:
```

**prometheus.yml**:
```yaml
global:
  scrape_interval: 30s

scrape_configs:
  - job_name: 'amazing-gitlab-exporter'
    static_configs:
      - targets: ['exporter:8080']
```

**Usage**:
```bash
export GITLAB_TOKEN=glpat-xxxxxxxxxxxxxxxxxxxx
docker-compose up -d
```

Access:
- Exporter metrics: http://localhost:8080/metrics
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 (admin/admin)

---

## GitHub Actions CI/CD

### Test Workflow (`.github/workflows/test.yml`)

Runs on: Pull requests, pushes to `main`

```yaml
name: Test

on:
  push:
    branches: [main]
  pull_request:

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v4
        with:
          version: latest

  test:
    runs-on: ubuntu-latest
    services:
      redis:
        image: redis:7-alpine
        ports:
          - 6379:6379
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Run tests
        run: |
          go test -v -race -coverprofile=coverage.out ./...
          go tool cover -html=coverage.out -o coverage.html
      - name: Upload coverage
        uses: codecov/codecov-action@v4
        with:
          files: ./coverage.out

  integration:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Build binary
        run: go build -o age ./cmd/amazing-gitlab-exporter
      - name: Run integration tests
        run: |
          ./age run --config configs/example.yml &
          sleep 10
          curl -f http://localhost:8080/health
          curl -f http://localhost:8080/metrics | grep age_
          kill %1

  docker:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Build Docker image
        run: docker build -t amazing-gitlab-exporter:test .
      - name: Test Docker image
        run: |
          docker run -d --name test-exporter \
            -e AGE_GITLAB_URL=https://gitlab.com \
            -e AGE_GITLAB_TOKEN=fake-token \
            amazing-gitlab-exporter:test
          sleep 5
          docker logs test-exporter
```

### Release Workflow (`.github/workflows/release.yml`)

Runs on: Tag push matching `v*` (e.g., `v1.0.0`)

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write
  packages: write

jobs:
  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v5
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

  docker:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Set up QEMU
        uses: docker/setup-qemu-action@v3
      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3
      - name: Login to GitHub Container Registry
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - name: Login to Docker Hub
        uses: docker/login-action@v3
        with:
          username: ${{ secrets.DOCKERHUB_USERNAME }}
          password: ${{ secrets.DOCKERHUB_TOKEN }}
      - name: Extract metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: |
            ghcr.io/${{ github.repository }}
            docker.io/${{ github.repository }}
          tags: |
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=semver,pattern={{major}}
            type=raw,value=latest,enable={{is_default_branch}}
      - name: Build and push
        uses: docker/build-push-action@v5
        with:
          context: .
          platforms: linux/amd64,linux/arm64
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

**GoReleaser config** (`.goreleaser.yml`):
```yaml
before:
  hooks:
    - go mod tidy
    - go generate ./...

builds:
  - env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    main: ./cmd/amazing-gitlab-exporter
    ldflags:
      - -s -w -X main.version={{.Version}} -X main.commit={{.Commit}}

archives:
  - format: tar.gz
    name_template: >-
      {{ .ProjectName }}_
      {{- title .Os }}_
      {{- if eq .Arch "amd64" }}x86_64
      {{- else }}{{ .Arch }}{{ end }}

checksum:
  name_template: 'checksums.txt'

changelog:
  sort: asc
  filters:
    exclude:
      - '^docs:'
      - '^test:'
```

---

## Implementation Plan

### Phase 1: Foundation (Est. 3-4 days)

**Goal**: Working exporter with basic pipeline & job metrics

Tasks:
- [ ] Initialize Go module and project structure
- [ ] Implement config package (YAML/env/CLI parsing, validation)
- [ ] Implement GitLab client (REST + GraphQL basics, rate limiting)
- [ ] Implement pipeline collector (with histograms)
- [ ] Implement job collector (with histograms)
- [ ] Implement HTTP server (/metrics, /health, /ready)
- [ ] Create Dockerfile
- [ ] Create minimal config example
- [ ] Unit tests for core packages

**Deliverable**: Exporter exports pipeline & job metrics with source labels

### Phase 2: Child Pipelines & Environments (Est. 2-3 days)

**Goal**: Full child/remote pipeline support + deployments

Tasks:
- [ ] Implement bridge job discovery for child pipelines
- [ ] Implement recursive child pipeline fetching
- [ ] Add child pipeline metrics with parent labels
- [ ] Implement environment/deployment collector
- [ ] Implement test report collector
- [ ] Add webhook receiver for push events
- [ ] Integration tests for child pipeline discovery

**Deliverable**: Complete CI/CD pipeline visibility including nested pipelines

### Phase 3: MR & Code Review Analytics (Est. 2-3 days)

**Goal**: Developer productivity metrics

Tasks:
- [ ] Implement MR analytics collector
  - [ ] Time to merge calculations
  - [ ] Time to first review
  - [ ] Review cycle counting
  - [ ] MR size tracking
- [ ] Implement code review analytics collector
  - [ ] Reviewer turnaround time
  - [ ] Approval tracking
- [ ] Implement repository analytics collector
  - [ ] Language detection
  - [ ] Commit frequency
  - [ ] Coverage tracking
- [ ] Implement contributor analytics collector
- [ ] Add webhook receiver for MR events

**Deliverable**: Full developer productivity metrics

### Phase 4: DORA & Value Stream (Est. 1-2 days)

**Goal**: Enterprise analytics (Premium/Ultimate)

Tasks:
- [ ] Implement tier detection (probe DORA/VSA endpoints)
- [ ] Implement DORA metrics collector
  - [ ] Deployment frequency
  - [ ] Lead time for changes
  - [ ] Time to restore service
  - [ ] Change failure rate
- [ ] Implement Value Stream Analytics collector
- [ ] Add `age_gitlab_tier` and `age_collector_enabled` metrics
- [ ] Test graceful degradation on Free tier

**Deliverable**: Enterprise-grade analytics with tier auto-detection

### Phase 5: High Availability (Est. 2 days)

**Goal**: Multi-instance support for large-scale deployments

Tasks:
- [ ] Implement store interface + memory store
- [ ] Implement Redis store
- [ ] Implement distributed rate limiting (Redis-based)
- [ ] Implement distributed task deduplication
- [ ] Add HA configuration options
- [ ] Test multi-replica scenario

**Deliverable**: Production-ready HA mode

### Phase 6: Grafana Dashboards (Est. 2 days)

**Goal**: Complete observability with pre-built dashboards

Tasks:
- [ ] Create Pipeline Overview dashboard
- [ ] Create Job Performance dashboard
- [ ] Create Merge Request Analytics dashboard
- [ ] Create DORA Metrics dashboard
- [ ] Create CI/CD Analytics dashboard
- [ ] Create Repository & Contributors dashboard
- [ ] Test dashboard imports and queries
- [ ] Create datasource provisioning config

**Deliverable**: 6 production-ready Grafana dashboards

### Phase 7: CI/CD & Documentation (Est. 1-2 days)

**Goal**: Automated testing and releases

Tasks:
- [ ] Create GitHub Actions test workflow
- [ ] Create GitHub Actions release workflow
- [ ] Configure GoReleaser
- [ ] Create docker-compose for local dev
- [ ] Write comprehensive README
- [ ] Write configuration documentation
- [ ] Write deployment guides (Docker, K8s)
- [ ] Create example configs (minimal, full)
- [ ] Add architecture diagrams

**Deliverable**: Complete project with CI/CD and documentation

### Phase 8: Testing & Optimization (Est. 2 days)

**Goal**: Production readiness

Tasks:
- [ ] Integration test suite (docker-compose based)
- [ ] Load testing (high project/pipeline count)
- [ ] Memory profiling and optimization
- [ ] Rate limit tuning
- [ ] Error handling review
- [ ] Security audit (token handling, injection risks)
- [ ] Performance benchmarks

**Deliverable**: Production-tested, optimized exporter

---

## Testing Strategy

### Unit Tests

- **Coverage target**: >80%
- **Packages**: All internal packages
- **Mocking**: Use `httptest.Server` for GitLab API mocks
- **Race detection**: Run with `-race` flag

Example:
```go
func TestPipelineCollector(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode([]gitlab.Pipeline{
            {ID: 123, Status: "success", Duration: 120},
        })
    }))
    defer server.Close()
    
    client := gitlab.NewClient(server.URL, "fake-token")
    collector := NewPipelineCollector(client, config)
    
    metrics, err := collector.Collect()
    assert.NoError(t, err)
    assert.Len(t, metrics, 1)
}
```

### Integration Tests

**docker-compose test stack**:
```yaml
services:
  gitlab-mock:
    image: stoplight/prism:latest
    command: mock -h 0.0.0.0 /gitlab-api.yml
    volumes:
      - ./test/gitlab-api.yml:/gitlab-api.yml
  
  exporter:
    build: .
    environment:
      - AGE_GITLAB_URL=http://gitlab-mock:4010
      - AGE_GITLAB_TOKEN=test-token
    depends_on:
      - gitlab-mock
```

**Test script**:
```bash
#!/bin/bash
docker-compose -f docker-compose.test.yml up -d
sleep 10

# Test health endpoint
curl -f http://localhost:8080/health || exit 1

# Test metrics endpoint
curl -f http://localhost:8080/metrics | grep age_pipeline_duration_seconds || exit 1

# Test specific metrics exist
curl -s http://localhost:8080/metrics | grep -q age_child_pipeline_duration_seconds || exit 1

docker-compose -f docker-compose.test.yml down
```

### Load Testing

**Scenario**: 1000 projects, 10 pipelines/project, 50 jobs/pipeline

```bash
# Generate test config with 1000 projects
go run ./test/generate-config.go --projects 1000 > test-config.yml

# Run exporter
./amazing-gitlab-exporter run --config test-config.yml &

# Monitor memory usage
while true; do
  ps aux | grep amazing-gitlab-exporter | grep -v grep
  sleep 5
done
```

**Success criteria**:
- Memory usage < 512MB under 1000-project load
- API rate limit respected (no 429 errors)
- Scrape time < 30s for 1000 projects

### Dashboard Testing

**Process**:
1. Import each dashboard JSON into Grafana
2. Configure Prometheus datasource
3. Set dashboard variables (project, ref, etc.)
4. Verify all panels render without errors
5. Verify queries return data (using test exporter)
6. Test filtering and time range changes

---

## Security Considerations

### Token Handling

- **Never log tokens**: Redact tokens in all log output and `/config` endpoint
- **Environment variables**: Prefer `AGE_GITLAB_TOKEN` over config file for sensitive deployments
- **Kubernetes secrets**: Use Secret resources, not ConfigMaps
- **Token scopes**: Document minimum required scopes (`api`, `read_repository`)

### Rate Limiting

- **Respect GitLab limits**: Default 10 req/s is conservative (GitLab.com: 2000 req/10min per user)
- **Backoff on 429**: Implement exponential backoff if rate-limited
- **Distributed limiting**: Prevent multiple replicas from exceeding combined limit

### API Injection

- **Input validation**: Validate all user-provided patterns (regexp, project names)
- **Parameterized queries**: Use GitLab client's parameter encoding
- **No shell execution**: Never execute shell commands with user input

### TLS Verification

- **Default: enabled**: Always verify TLS unless explicitly disabled
- **Custom CA support**: Allow custom CA certificates for self-hosted GitLab
- **Warn on disabled**: Log warning if `enable_tls_verify: false`

---

## Performance Targets

| Metric | Target | Notes |
|--------|--------|-------|
| **Docker image size** | < 20MB | Using scratch base image |
| **Memory usage** | < 256MB | For 100 projects, single instance |
| **Memory usage (large)** | < 512MB | For 1000 projects |
| **Scrape duration** | < 30s | Full metric collection |
| **API calls per minute** | Configurable | Default: 600 (10/s) |
| **Startup time** | < 10s | First metrics available |
| **CPU usage** | < 0.5 cores | Steady state |

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| **GitLab API rate limiting** | High | Respect headers, configurable limits, exponential backoff, GraphQL batching |
| **Memory exhaustion** | High | Pagination, incremental fetching, max items per project, garbage collection |
| **Stale metrics** | Medium | Configurable polling intervals, webhook acceleration, timestamp tracking |
| **Child pipeline discovery failing** | Medium | Graceful error handling, skip failed child pipelines, log errors |
| **Redis unavailable (HA mode)** | Medium | Fallback to single-instance mode, health check Redis connection |
| **Tier detection false positive** | Low | Conservative detection, allow manual tier override in config |
| **GraphQL query breaking changes** | Low | Use versioned GitLab client, fallback to REST on GraphQL errors |

---

## Future Enhancements (Not in MVP)

- **Helm chart** for Kubernetes deployment
- **OpenTelemetry tracing** for API call visibility
- **Alerting rules** (Prometheus rules + alert templates)
- **Slack/Discord webhook** notifications for critical events
- **Artifact analytics** (artifact size trends, storage usage)
- **Runner analytics** (runner utilization, queue times)
- **Cost analytics** (compute minutes tracking for SaaS)
- **Multi-GitLab instance support** (single exporter monitoring multiple GitLab instances)
- **GraphQL subscriptions** for real-time updates (when GitLab supports)
- **SQL export mode** for data warehousing (write metrics to PostgreSQL/BigQuery)

---

## Success Criteria Checklist

At project completion, verify:

- [ ] Exports 100+ unique Prometheus metrics
- [ ] Supports GitLab Free, Premium, and Ultimate tiers
- [ ] Auto-detects GitLab tier and gracefully degrades
- [ ] Tracks child/remote pipelines with parent relationships
- [ ] Uses histograms for pipeline/job durations (P50/P95/P99 support)
- [ ] Hybrid REST + GraphQL reduces API calls by 60%+
- [ ] Works with GitLab SaaS and self-hosted
- [ ] Docker image < 20MB, multi-arch (amd64/arm64)
- [ ] Supports single-instance and HA (Redis) modes
- [ ] Configurable via YAML, env vars, and CLI flags
- [ ] 6 pre-built Grafana dashboards included
- [ ] GitHub Actions CI/CD for testing and releases
- [ ] Comprehensive documentation (README, config docs, deployment guides)
- [ ] >80% unit test coverage
- [ ] Integration tests pass
- [ ] Load tested with 1000 projects
- [ ] Security audit completed

---

## Appendix A: Comparison with Existing Exporter

| Feature | gitlab-ci-pipelines-exporter | amazing-gitlab-exporter |
|---------|------------------------------|-------------------------|
| **Language** | Go | Go |
| **Metrics count** | ~30 | 100+ |
| **Duration metrics** | Gauge (single value) | Histogram (percentiles) |
| **Pipeline source tracking** | No | Yes (push/MR/schedule/trigger) |
| **Child pipeline support** | Limited (jobs only) | Full (pipelines + jobs with parent context) |
| **MR analytics** | No | Yes |
| **Code review analytics** | No | Yes |
| **DORA metrics** | No | Yes (Ultimate) |
| **Value Stream Analytics** | No | Yes (Premium) |
| **Repository analytics** | No | Yes |
| **Contributor analytics** | No | Yes |
| **API strategy** | REST only | REST + GraphQL hybrid |
| **Incremental fetching** | No | Yes (`updated_after`) |
| **Tier detection** | No | Yes (auto-detect) |
| **HA mode** | Yes (Redis) | Yes (Redis) |
| **Webhooks** | Yes | Yes |
| **Grafana dashboards** | No | 6 pre-built |
| **Docker image size** | ~30MB | <20MB |

---

## Appendix B: GitLab API Endpoints Used

| Endpoint | Method | Purpose | Tier |
|----------|--------|---------|------|
| `/api/v4/version` | GET | Validate connection, get version | Free |
| `/api/v4/projects` | GET | List projects | Free |
| `/api/v4/projects/:id` | GET | Get project details | Free |
| `/api/v4/projects/:id/pipelines` | GET | List pipelines | Free |
| `/api/v4/projects/:id/pipelines/:pid` | GET | Get pipeline details | Free |
| `/api/v4/projects/:id/pipelines/:pid/jobs` | GET | List pipeline jobs | Free |
| `/api/v4/projects/:id/pipelines/:pid/bridges` | GET | List bridge/trigger jobs | Free |
| `/api/v4/projects/:id/pipelines/:pid/test_report` | GET | Get test report | Free |
| `/api/v4/projects/:id/pipelines/:pid/variables` | GET | Get pipeline variables | Free |
| `/api/v4/projects/:id/merge_requests` | GET | List merge requests | Free |
| `/api/v4/projects/:id/merge_requests/:iid` | GET | Get MR details | Free |
| `/api/v4/projects/:id/environments` | GET | List environments | Free |
| `/api/v4/projects/:id/deployments` | GET | List deployments | Free |
| `/api/v4/projects/:id/repository/languages` | GET | Get language breakdown | Free |
| `/api/v4/projects/:id/repository/commits` | GET | List commits | Free |
| `/api/v4/projects/:id/dora/metrics` | GET | Get DORA metrics | Ultimate |
| `/api/v4/projects/:id/analytics/value_stream_analytics` | GET | Get VSA metrics | Premium |
| `/api/graphql` | POST | GraphQL queries | Free |

---

## Contact & Contribution

- **Repository**: TBD (will be created on GitHub)
- **License**: Apache-2.0
- **Issues**: GitHub Issues
- **Discussions**: GitHub Discussions
- **Pull Requests**: Welcome! See CONTRIBUTING.md

---

**End of Project Plan**
