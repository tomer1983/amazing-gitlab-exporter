# Changelog

All notable changes to the amazing-gitlab-exporter Helm chart will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.0] - 2026-02-16

### Added
- Initial Helm chart release for Amazing GitLab Exporter
- Full Kubernetes 1.25+ and OpenShift 4.12+ support
- Security hardening with Restricted Pod Security Standards
  - Non-root containers with `runAsNonRoot: true`
  - Read-only root filesystem
  - Dropped all capabilities
  - Seccomp profile `RuntimeDefault`
- Optional sub-charts for complete monitoring stack
  - Bitnami Redis ~19.6.0 for HA state persistence
  - Prometheus Community ~25.27.0 for metrics collection
  - Grafana ~8.6.0 for visualization
- Monitoring integration templates
  - ServiceMonitor for Prometheus Operator
  - PrometheusRule with 6 default alerts
  - 7 pre-configured Grafana dashboards (CI/CD, DORA, Jobs, MRs, Pipelines, Repositories, Exporter Activity)
- Kubernetes resources
  - Deployment with rolling update strategy
  - ClusterIP Service
  - ConfigMap for exporter configuration
  - Secret for sensitive credentials
  - Optional ServiceAccount with IAM role annotation support
- Optional resources
  - Ingress (networking.k8s.io/v1) with multi-host and TLS support
  - OpenShift Route with TLS termination options
  - HorizontalPodAutoscaler (autoscaling/v2) with behavior config
  - PodDisruptionBudget for HA deployments
  - NetworkPolicy for ingress/egress control
- Configuration
  - JSON Schema validation for values (values.schema.json)
  - Named templates in _helpers.tpl for reusability
  - Comprehensive default values with inline documentation
- Testing
  - Helm test pods (health check, metrics validation)
  - helm-unittest test suites for 10 templates
  - CI values files for 5 deployment scenarios (default, HA, full-stack, OpenShift, full)
- Documentation
  - Comprehensive chart README with usage examples
  - Troubleshooting guide
  - Security best practices
- CI/CD
  - GitHub Actions workflow for lint, template test, unit test
  - Automated chart packaging and OCI publishing to GHCR
  - Security scanning with Checkov and Trivy
  - GitHub Release automation on chart-* tags
- OpenShift compatibility
  - Route resource with `.Capabilities.APIVersions.Has` detection
  - Dynamic UID support (no hardcoded `runAsUser`)
  - SCC restricted-v2 compatible security contexts

### Changed
- N/A (initial release)

### Deprecated
- N/A (initial release)

### Removed
- N/A (initial release)

### Fixed
- N/A (initial release)

### Security
- Chart follows CIS Kubernetes Benchmarks for security hardening
- All containers run as non-root with minimal privileges
- Network policies restrict traffic to required endpoints only
- Secrets never exposed in logs or ConfigMaps

[Unreleased]: https://github.com/tomer1983/amazing-gitlab-exporter/compare/chart-1.0.0...HEAD
[1.0.0]: https://github.com/tomer1983/amazing-gitlab-exporter/releases/tag/chart-1.0.0
