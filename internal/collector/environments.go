package collector

import (
	"context"
	"fmt"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/internal/config"
	gitlabclient "github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/internal/gitlab"
)

// EnvironmentsCollector fetches environment and deployment data from the
// GitLab API and exposes Prometheus metrics for deployment durations, statuses,
// counts, staleness, and environment information.
type EnvironmentsCollector struct {
	client   *gitlabclient.Client
	config   config.EnvironmentsCollectorConfig
	projects []string
	mu       sync.RWMutex

	deployDuration *prometheus.HistogramVec
	deployStatus   *prometheus.GaugeVec
	deployCount    *prometheus.CounterVec
	behindCommits  *prometheus.GaugeVec
	behindDuration *prometheus.GaugeVec
	info           *prometheus.GaugeVec

	logger *logrus.Entry
}

// compile-time interface check
var _ Collector = (*EnvironmentsCollector)(nil)

// NewEnvironmentsCollector creates an EnvironmentsCollector wired to the given
// GitLab client and configuration.
func NewEnvironmentsCollector(client *gitlabclient.Client, cfg config.EnvironmentsCollectorConfig, projects []string) *EnvironmentsCollector {
	buckets := prometheus.DefBuckets // environments collector uses default buckets

	return &EnvironmentsCollector{
		client:   client,
		config:   cfg,
		projects: projects,
		logger:   logrus.WithField("collector", "environments"),

		deployDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "age_environment_deployment_duration_seconds",
			Help:    "Deployment duration in seconds.",
			Buckets: buckets,
		}, []string{"project", "environment"}),

		deployStatus: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "age_environment_deployment_status",
			Help: "Deployment status (1 = current status matches label, 0 otherwise).",
		}, []string{"project", "environment", "status"}),

		deployCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "age_environment_deployment_count",
			Help: "Total deployments.",
		}, []string{"project", "environment"}),

		behindCommits: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "age_environment_behind_commits",
			Help: "Number of commits behind the target branch.",
		}, []string{"project", "environment"}),

		behindDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "age_environment_behind_duration_seconds",
			Help: "Time since last deployment in seconds.",
		}, []string{"project", "environment"}),

		info: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "age_environment_info",
			Help: "Informational metric about the environment (always 1).",
		}, []string{"project", "environment", "tier"}),
	}
}

// Name returns the collector name.
func (c *EnvironmentsCollector) Name() string { return "environments" }

// Enabled reports whether this collector is active.
func (c *EnvironmentsCollector) Enabled() bool { return c.config.Enabled }

// SetProjects updates the list of tracked project paths.
func (c *EnvironmentsCollector) SetProjects(projects []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projects = projects
}

// Describe sends all metric descriptors to ch.
func (c *EnvironmentsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.deployDuration.Describe(ch)
	c.deployStatus.Describe(ch)
	c.deployCount.Describe(ch)
	c.behindCommits.Describe(ch)
	c.behindDuration.Describe(ch)
	c.info.Describe(ch)
}

// Collect sends all current metric values to ch.
func (c *EnvironmentsCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	c.deployDuration.Collect(ch)
	c.deployStatus.Collect(ch)
	c.deployCount.Collect(ch)
	c.behindCommits.Collect(ch)
	c.behindDuration.Collect(ch)
	c.info.Collect(ch)
}

// Run fetches environment and deployment data for every tracked project.
func (c *EnvironmentsCollector) Run(ctx context.Context) error {
	c.mu.RLock()
	projects := make([]string, len(c.projects))
	copy(projects, c.projects)
	c.mu.RUnlock()

	for _, project := range projects {
		if err := c.collectProject(ctx, project); err != nil {
			c.logger.WithFields(logrus.Fields{
				"project": project,
				"error":   err,
			}).Error("failed to collect environments")
		}
	}
	return nil
}

// collectProject fetches environments and their latest deployments for a single project.
func (c *EnvironmentsCollector) collectProject(ctx context.Context, project string) error {
	rest := c.client.REST()

	envOpts := &gitlab.ListEnvironmentsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 50},
	}

	envs, _, err := rest.Environments.ListEnvironments(project, envOpts, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("list environments for %s: %w", project, err)
	}

	for _, env := range envs {
		// Optionally skip stopped environments.
		if c.config.ExcludeStopped && env.State == "stopped" {
			continue
		}

		envName := env.Name
		tier := environmentTier(env)

		c.mu.Lock()
		c.info.WithLabelValues(project, envName, tier).Set(1)
		c.mu.Unlock()

		// Fetch deployments for this environment.
		c.collectDeployments(ctx, project, env)
	}
	return nil
}

// collectDeployments fetches the latest deployments for an environment.
func (c *EnvironmentsCollector) collectDeployments(ctx context.Context, project string, env *gitlab.Environment) {
	rest := c.client.REST()
	envName := env.Name

	depOpts := &gitlab.ListProjectDeploymentsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 10},
		Environment: gitlab.Ptr(envName),
		OrderBy:     gitlab.Ptr("created_at"),
		Sort:        gitlab.Ptr("desc"),
	}

	deployments, _, err := rest.Deployments.ListProjectDeployments(project, depOpts, gitlab.WithContext(ctx))
	if err != nil {
		c.logger.WithFields(logrus.Fields{
			"project":     project,
			"environment": envName,
			"error":       err,
		}).Warn("failed to list deployments")
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, d := range deployments {
		status := d.Status

		c.deployStatus.WithLabelValues(project, envName, status).Set(1)
		c.deployCount.WithLabelValues(project, envName).Inc()

		// Duration: use the deployable (job) duration if available.
		if d.Deployable.ID != 0 && d.Deployable.Duration > 0 {
			c.deployDuration.WithLabelValues(project, envName).Observe(d.Deployable.Duration)
		}
	}
}

// environmentTier extracts the tier string from an environment.
func environmentTier(env *gitlab.Environment) string {
	if env.Tier != "" {
		return env.Tier
	}
	return "unknown"
}
