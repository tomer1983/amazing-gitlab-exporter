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

// PipelinesCollector fetches pipeline data from the GitLab API and exposes
// Prometheus histogram and gauge/counter metrics for pipeline durations,
// statuses, and child/remote pipelines.
type PipelinesCollector struct {
	client   *gitlabclient.Client
	config   config.PipelinesCollectorConfig
	projects []string
	mu       sync.RWMutex

	// --- primary pipeline metrics ---
	duration       *prometheus.HistogramVec
	queuedDuration *prometheus.HistogramVec
	status         *prometheus.GaugeVec
	runCount       *prometheus.CounterVec
	coverage       *prometheus.GaugeVec
	id             *prometheus.GaugeVec
	createdTS      *prometheus.GaugeVec

	// --- child pipeline metrics ---
	childDuration       *prometheus.HistogramVec
	childStatus         *prometheus.GaugeVec
	childRunCount       *prometheus.CounterVec
	childQueuedDuration *prometheus.HistogramVec

	logger *logrus.Entry
}

// compile-time interface check
var _ Collector = (*PipelinesCollector)(nil)

// NewPipelinesCollector creates a PipelinesCollector wired to the given GitLab
// client and configuration. histogram_buckets from config control duration
// histogram boundaries.
func NewPipelinesCollector(client *gitlabclient.Client, cfg config.PipelinesCollectorConfig, projects []string) *PipelinesCollector {
	buckets := cfg.HistogramBuckets
	if len(buckets) == 0 {
		buckets = prometheus.DefBuckets
	}

	c := &PipelinesCollector{
		client:   client,
		config:   cfg,
		projects: projects,
		logger:   logrus.WithField("collector", "pipelines"),

		// --- primary ---
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "age_pipeline_duration_seconds",
			Help:    "Pipeline execution duration in seconds.",
			Buckets: buckets,
		}, []string{"project", "ref", "kind", "source", "status"}),

		queuedDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "age_pipeline_queued_duration_seconds",
			Help:    "Time a pipeline spent queued before execution in seconds.",
			Buckets: buckets,
		}, []string{"project", "ref", "kind", "source"}),

		status: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "age_pipeline_status",
			Help: "Pipeline status (1 = current status matches label, 0 otherwise).",
		}, []string{"project", "ref", "kind", "source", "status"}),

		runCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "age_pipeline_run_count",
			Help: "Total pipeline runs.",
		}, []string{"project", "ref", "kind", "source"}),

		coverage: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "age_pipeline_coverage",
			Help: "Code coverage percentage reported by the pipeline.",
		}, []string{"project", "ref", "kind"}),

		id: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "age_pipeline_id",
			Help: "Latest pipeline ID.",
		}, []string{"project", "ref", "kind"}),

		createdTS: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "age_pipeline_created_timestamp",
			Help: "Pipeline creation timestamp (unix epoch seconds).",
		}, []string{"project", "ref", "kind"}),

		// --- child pipelines ---
		childDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "age_child_pipeline_duration_seconds",
			Help:    "Child/triggered pipeline execution duration in seconds.",
			Buckets: buckets,
		}, []string{"project", "ref", "parent_project", "parent_ref", "bridge_name"}),

		childStatus: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "age_child_pipeline_status",
			Help: "Child/triggered pipeline status.",
		}, []string{"project", "ref", "parent_project", "parent_ref", "bridge_name", "status"}),

		childRunCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "age_child_pipeline_run_count",
			Help: "Total child/triggered pipeline executions.",
		}, []string{"project", "ref", "parent_project", "parent_ref", "bridge_name"}),

		childQueuedDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "age_child_pipeline_queued_duration_seconds",
			Help:    "Child/triggered pipeline queue time in seconds.",
			Buckets: buckets,
		}, []string{"project", "ref", "parent_project", "parent_ref", "bridge_name"}),
	}
	return c
}

// Name returns the collector name.
func (c *PipelinesCollector) Name() string { return "pipelines" }

// Enabled reports whether this collector is active.
func (c *PipelinesCollector) Enabled() bool { return c.config.Enabled }

// SetProjects updates the list of tracked project paths.
func (c *PipelinesCollector) SetProjects(projects []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projects = projects
}

// Describe sends all metric descriptors to ch.
func (c *PipelinesCollector) Describe(ch chan<- *prometheus.Desc) {
	c.duration.Describe(ch)
	c.queuedDuration.Describe(ch)
	c.status.Describe(ch)
	c.runCount.Describe(ch)
	c.coverage.Describe(ch)
	c.id.Describe(ch)
	c.createdTS.Describe(ch)
	c.childDuration.Describe(ch)
	c.childStatus.Describe(ch)
	c.childRunCount.Describe(ch)
	c.childQueuedDuration.Describe(ch)
}

// Collect sends all current metric values to ch.
func (c *PipelinesCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	c.duration.Collect(ch)
	c.queuedDuration.Collect(ch)
	c.status.Collect(ch)
	c.runCount.Collect(ch)
	c.coverage.Collect(ch)
	c.id.Collect(ch)
	c.createdTS.Collect(ch)
	c.childDuration.Collect(ch)
	c.childStatus.Collect(ch)
	c.childRunCount.Collect(ch)
	c.childQueuedDuration.Collect(ch)
}

// Run fetches pipelines for every tracked project and updates metric state.
func (c *PipelinesCollector) Run(ctx context.Context) error {
	c.mu.RLock()
	projects := make([]string, len(c.projects))
	copy(projects, c.projects)
	c.mu.RUnlock()

	for _, project := range projects {
		if err := c.collectProject(ctx, project); err != nil {
			c.logger.WithFields(logrus.Fields{
				"project": project,
				"error":   err,
			}).Error("failed to collect pipelines")
			// Continue with remaining projects rather than aborting.
		}
	}
	return nil
}

// collectProject fetches pipeline data for a single project.
func (c *PipelinesCollector) collectProject(ctx context.Context, project string) error {
	rest := c.client.REST()

	opt := &gitlab.ListProjectPipelinesOptions{
		ListOptions: gitlab.ListOptions{PerPage: c.config.MaxPipelinesPerRef},
	}

	pipelines, _, err := rest.Pipelines.ListProjectPipelines(project, opt, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("list pipelines for %s: %w", project, err)
	}

	for _, p := range pipelines {
		pipeline, _, err := rest.Pipelines.GetPipeline(project, p.ID, gitlab.WithContext(ctx))
		if err != nil {
			c.logger.WithFields(logrus.Fields{
				"project":  project,
				"pipeline": p.ID,
				"error":    err,
			}).Warn("failed to get pipeline details")
			continue
		}

		c.recordPipeline(project, pipeline)

		// Optionally discover child pipelines via bridge jobs.
		if c.config.IncludeChildPipelines {
			c.collectChildPipelines(ctx, project, pipeline)
		}
	}
	return nil
}

// recordPipeline updates all primary pipeline metrics for a single pipeline.
func (c *PipelinesCollector) recordPipeline(project string, p *gitlab.Pipeline) {
	ref := p.Ref
	kind := pipelineKind(p)
	source := p.Source
	status := p.Status

	c.mu.Lock()
	defer c.mu.Unlock()

	if p.Duration > 0 {
		c.duration.WithLabelValues(project, ref, kind, source, status).Observe(float64(p.Duration))
	}
	if p.QueuedDuration > 0 {
		c.queuedDuration.WithLabelValues(project, ref, kind, source).Observe(float64(p.QueuedDuration))
	}

	c.status.WithLabelValues(project, ref, kind, source, status).Set(1)
	c.runCount.WithLabelValues(project, ref, kind, source).Inc()

	if p.Coverage != "" {
		var cov float64
		if _, err := fmt.Sscanf(p.Coverage, "%f", &cov); err == nil {
			c.coverage.WithLabelValues(project, ref, kind).Set(cov)
		}
	}

	c.id.WithLabelValues(project, ref, kind).Set(float64(p.ID))

	if p.CreatedAt != nil {
		c.createdTS.WithLabelValues(project, ref, kind).Set(float64(p.CreatedAt.Unix()))
	}
}

// collectChildPipelines discovers child/triggered pipelines through bridge
// jobs and records their metrics.
func (c *PipelinesCollector) collectChildPipelines(ctx context.Context, project string, parent *gitlab.Pipeline) {
	rest := c.client.REST()

	bridges, _, err := rest.Jobs.ListPipelineBridges(project, parent.ID, &gitlab.ListJobsOptions{}, gitlab.WithContext(ctx))
	if err != nil {
		c.logger.WithFields(logrus.Fields{
			"project":  project,
			"pipeline": parent.ID,
			"error":    err,
		}).Warn("failed to list bridge jobs")
		return
	}

	for _, bridge := range bridges {
		if bridge.DownstreamPipeline == nil {
			continue
		}

		dp := bridge.DownstreamPipeline
		childProject := ""
		if dp.ProjectID != 0 {
			childProject = fmt.Sprintf("%d", dp.ProjectID)
		}
		childRef := dp.Ref
		parentProject := project
		parentRef := parent.Ref
		bridgeName := bridge.Name

		// Fetch the full pipeline to get Duration/QueuedDuration
		// (PipelineInfo from bridge does not include these fields).
		var duration, queuedDuration float64
		if childProject != "" {
			if fullPipeline, _, err := c.client.REST().Pipelines.GetPipeline(childProject, dp.ID); err == nil {
				duration = float64(fullPipeline.Duration)
				queuedDuration = float64(fullPipeline.QueuedDuration)
			}
		}

		c.mu.Lock()

		if duration > 0 {
			c.childDuration.WithLabelValues(childProject, childRef, parentProject, parentRef, bridgeName).Observe(duration)
		}
		if queuedDuration > 0 {
			c.childQueuedDuration.WithLabelValues(childProject, childRef, parentProject, parentRef, bridgeName).Observe(queuedDuration)
		}
		c.childStatus.WithLabelValues(childProject, childRef, parentProject, parentRef, bridgeName, dp.Status).Set(1)
		c.childRunCount.WithLabelValues(childProject, childRef, parentProject, parentRef, bridgeName).Inc()

		c.mu.Unlock()
	}
}

// pipelineKind returns a human-readable kind string for the pipeline.
func pipelineKind(p *gitlab.Pipeline) string {
	switch p.Source {
	case "parent_pipeline":
		return "child"
	case "trigger", "pipeline":
		return "trigger"
	case "merge_request_event":
		return "merge_request"
	case "schedule":
		return "schedule"
	default:
		return "branch"
	}
}
