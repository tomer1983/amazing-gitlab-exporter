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

// JobsCollector fetches job-level data from the GitLab API and exposes
// Prometheus histogram and gauge/counter metrics for job durations, statuses,
// artifact sizes, and runner information.
type JobsCollector struct {
	client   *gitlabclient.Client
	config   config.JobsCollectorConfig
	projects []string
	mu       sync.RWMutex

	duration       *prometheus.HistogramVec
	queuedDuration *prometheus.HistogramVec
	status         *prometheus.GaugeVec
	runCount       *prometheus.CounterVec
	artifactSize   *prometheus.GaugeVec

	logger *logrus.Entry
}

// compile-time interface check
var _ Collector = (*JobsCollector)(nil)

// NewJobsCollector creates a JobsCollector wired to the given GitLab client
// and configuration.
func NewJobsCollector(client *gitlabclient.Client, cfg config.JobsCollectorConfig, projects []string) *JobsCollector {
	buckets := cfg.HistogramBuckets
	if len(buckets) == 0 {
		buckets = prometheus.DefBuckets
	}

	return &JobsCollector{
		client:   client,
		config:   cfg,
		projects: projects,
		logger:   logrus.WithField("collector", "jobs"),

		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "age_job_duration_seconds",
			Help:    "Job execution duration in seconds.",
			Buckets: buckets,
		}, []string{"project", "ref", "stage", "job_name", "runner_type", "status"}),

		queuedDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "age_job_queued_duration_seconds",
			Help:    "Time a job spent queued before execution in seconds.",
			Buckets: buckets,
		}, []string{"project", "ref", "stage", "job_name"}),

		status: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "age_job_status",
			Help: "Job status (1 = current status matches label, 0 otherwise).",
		}, []string{"project", "ref", "stage", "job_name", "status", "failure_reason"}),

		runCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "age_job_run_count",
			Help: "Total job executions.",
		}, []string{"project", "ref", "stage", "job_name"}),

		artifactSize: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "age_job_artifact_size_bytes",
			Help: "Job artifact size in bytes.",
		}, []string{"project", "ref", "stage", "job_name"}),
	}
}

// Name returns the collector name.
func (c *JobsCollector) Name() string { return "jobs" }

// Enabled reports whether this collector is active.
func (c *JobsCollector) Enabled() bool { return c.config.Enabled }

// SetProjects updates the list of tracked project paths.
func (c *JobsCollector) SetProjects(projects []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projects = projects
}

// Describe sends all metric descriptors to ch.
func (c *JobsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.duration.Describe(ch)
	c.queuedDuration.Describe(ch)
	c.status.Describe(ch)
	c.runCount.Describe(ch)
	c.artifactSize.Describe(ch)
}

// Collect sends all current metric values to ch.
func (c *JobsCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	c.duration.Collect(ch)
	c.queuedDuration.Collect(ch)
	c.status.Collect(ch)
	c.runCount.Collect(ch)
	c.artifactSize.Collect(ch)
}

// Run fetches job data for every tracked project and updates metrics.
func (c *JobsCollector) Run(ctx context.Context) error {
	c.mu.RLock()
	projects := make([]string, len(c.projects))
	copy(projects, c.projects)
	c.mu.RUnlock()

	for _, project := range projects {
		if err := c.collectProject(ctx, project); err != nil {
			c.logger.WithFields(logrus.Fields{
				"project": project,
				"error":   err,
			}).Error("failed to collect jobs")
		}
	}
	return nil
}

// collectProject fetches pipelines for a project then iterates their jobs.
func (c *JobsCollector) collectProject(ctx context.Context, project string) error {
	rest := c.client.REST()

	pipelines, _, err := rest.Pipelines.ListProjectPipelines(project, &gitlab.ListProjectPipelinesOptions{
		ListOptions: gitlab.ListOptions{PerPage: 20},
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("list pipelines for jobs in %s: %w", project, err)
	}

	for _, p := range pipelines {
		jobs, _, err := rest.Jobs.ListPipelineJobs(project, p.ID, &gitlab.ListJobsOptions{}, gitlab.WithContext(ctx))
		if err != nil {
			c.logger.WithFields(logrus.Fields{
				"project":  project,
				"pipeline": p.ID,
				"error":    err,
			}).Warn("failed to list pipeline jobs")
			continue
		}

		for _, job := range jobs {
			c.recordJob(project, job)
		}
	}
	return nil
}

// recordJob updates all job metrics for a single job.
func (c *JobsCollector) recordJob(project string, j *gitlab.Job) {
	ref := j.Ref
	stage := j.Stage
	name := j.Name
	status := j.Status
	runnerType := resolveRunnerType(j, c.config.IncludeRunnerDetails)

	failureReason := ""
	if j.FailureReason != "" {
		failureReason = j.FailureReason
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if j.Duration > 0 {
		c.duration.WithLabelValues(project, ref, stage, name, runnerType, status).Observe(j.Duration)
	}
	if j.QueuedDuration > 0 {
		c.queuedDuration.WithLabelValues(project, ref, stage, name).Observe(j.QueuedDuration)
	}

	c.status.WithLabelValues(project, ref, stage, name, status, failureReason).Set(1)
	c.runCount.WithLabelValues(project, ref, stage, name).Inc()

	// Sum artifact sizes.
	var totalArtifactSize float64
	for _, a := range j.Artifacts {
		totalArtifactSize += float64(a.Size)
	}
	if totalArtifactSize > 0 {
		c.artifactSize.WithLabelValues(project, ref, stage, name).Set(totalArtifactSize)
	}
}

// resolveRunnerType determines the runner type string from the job's Runner
// field. Values: "instance", "group", "project", "unknown".
func resolveRunnerType(j *gitlab.Job, includeDetails bool) string {
	if !includeDetails || j.Runner.ID == 0 {
		return "unknown"
	}
	if j.Runner.IsShared {
		return "instance"
	}
	// The GitLab API doesn't expose a direct group vs project flag on the
	// Runner struct in the job response. We use IsShared to distinguish
	// shared (instance) runners; all others are classified as "project"
	// unless a description heuristic is added later.
	return "project"
}
