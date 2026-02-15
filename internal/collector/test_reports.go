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

// TestReportsCollector fetches pipeline test reports from the GitLab API and
// exposes Prometheus metrics at the report, suite, and (optionally) individual
// test case levels.
type TestReportsCollector struct {
	client   *gitlabclient.Client
	config   config.TestReportsCollectorConfig
	projects []string
	mu       sync.RWMutex

	// report-level metrics
	totalTime    *prometheus.GaugeVec
	totalCount   *prometheus.GaugeVec
	successCount *prometheus.GaugeVec
	failedCount  *prometheus.GaugeVec
	skippedCount *prometheus.GaugeVec
	errorCount   *prometheus.GaugeVec

	// suite-level metrics
	suiteDuration *prometheus.GaugeVec
	suiteCount    *prometheus.GaugeVec

	// case-level metrics (enabled via config.IncludeTestCases)
	caseDuration *prometheus.HistogramVec
	caseStatus   *prometheus.GaugeVec

	logger *logrus.Entry
}

// compile-time interface check
var _ Collector = (*TestReportsCollector)(nil)

// NewTestReportsCollector creates a TestReportsCollector wired to the given
// GitLab client and configuration.
func NewTestReportsCollector(client *gitlabclient.Client, cfg config.TestReportsCollectorConfig, projects []string) *TestReportsCollector {
	caseBuckets := prometheus.DefBuckets

	return &TestReportsCollector{
		client:   client,
		config:   cfg,
		projects: projects,
		logger:   logrus.WithField("collector", "test_reports"),

		// --- report level ---
		totalTime: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "age_test_report_total_time_seconds",
			Help: "Total test execution time in seconds.",
		}, []string{"project", "ref"}),

		totalCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "age_test_report_total_count",
			Help: "Total number of tests in the report.",
		}, []string{"project", "ref"}),

		successCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "age_test_report_success_count",
			Help: "Number of successful tests.",
		}, []string{"project", "ref"}),

		failedCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "age_test_report_failed_count",
			Help: "Number of failed tests.",
		}, []string{"project", "ref"}),

		skippedCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "age_test_report_skipped_count",
			Help: "Number of skipped tests.",
		}, []string{"project", "ref"}),

		errorCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "age_test_report_error_count",
			Help: "Number of tests with errors.",
		}, []string{"project", "ref"}),

		// --- suite level ---
		suiteDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "age_test_suite_duration_seconds",
			Help: "Test suite execution duration in seconds.",
		}, []string{"project", "ref", "suite_name"}),

		suiteCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "age_test_suite_count",
			Help: "Number of tests in a suite.",
		}, []string{"project", "ref", "suite_name"}),

		// --- case level ---
		caseDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "age_test_case_duration_seconds",
			Help:    "Individual test case execution duration in seconds.",
			Buckets: caseBuckets,
		}, []string{"project", "ref", "suite", "case_name"}),

		caseStatus: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "age_test_case_status",
			Help: "Test case status (1 = current status matches label, 0 otherwise).",
		}, []string{"project", "ref", "suite", "case_name", "status"}),
	}
}

// Name returns the collector name.
func (c *TestReportsCollector) Name() string { return "test_reports" }

// Enabled reports whether this collector is active.
func (c *TestReportsCollector) Enabled() bool { return c.config.Enabled }

// SetProjects updates the list of tracked project paths.
func (c *TestReportsCollector) SetProjects(projects []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projects = projects
}

// Describe sends all metric descriptors to ch.
func (c *TestReportsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.totalTime.Describe(ch)
	c.totalCount.Describe(ch)
	c.successCount.Describe(ch)
	c.failedCount.Describe(ch)
	c.skippedCount.Describe(ch)
	c.errorCount.Describe(ch)
	c.suiteDuration.Describe(ch)
	c.suiteCount.Describe(ch)
	c.caseDuration.Describe(ch)
	c.caseStatus.Describe(ch)
}

// Collect sends all current metric values to ch.
func (c *TestReportsCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	c.totalTime.Collect(ch)
	c.totalCount.Collect(ch)
	c.successCount.Collect(ch)
	c.failedCount.Collect(ch)
	c.skippedCount.Collect(ch)
	c.errorCount.Collect(ch)
	c.suiteDuration.Collect(ch)
	c.suiteCount.Collect(ch)
	c.caseDuration.Collect(ch)
	c.caseStatus.Collect(ch)
}

// Run fetches test reports for every tracked project and updates metrics.
func (c *TestReportsCollector) Run(ctx context.Context) error {
	c.mu.RLock()
	projects := make([]string, len(c.projects))
	copy(projects, c.projects)
	c.mu.RUnlock()

	for _, project := range projects {
		if err := c.collectProject(ctx, project); err != nil {
			c.logger.WithFields(logrus.Fields{
				"project": project,
				"error":   err,
			}).Error("failed to collect test reports")
		}
	}
	return nil
}

// collectProject fetches recent pipelines and their test reports for a single project.
func (c *TestReportsCollector) collectProject(ctx context.Context, project string) error {
	rest := c.client.REST()

	pipelines, _, err := rest.Pipelines.ListProjectPipelines(project, &gitlab.ListProjectPipelinesOptions{
		ListOptions: gitlab.ListOptions{PerPage: 10},
	}, gitlab.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("list pipelines for test reports in %s: %w", project, err)
	}

	for _, p := range pipelines {
		report, _, err := rest.Pipelines.GetPipelineTestReport(project, p.ID, gitlab.WithContext(ctx))
		if err != nil {
			c.logger.WithFields(logrus.Fields{
				"project":  project,
				"pipeline": p.ID,
				"error":    err,
			}).Debug("no test report for pipeline (may be expected)")
			continue
		}

		c.recordReport(project, p.Ref, report)
	}
	return nil
}

// recordReport updates all test report metrics for a single pipeline's test report.
func (c *TestReportsCollector) recordReport(project, ref string, r *gitlab.PipelineTestReport) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.totalTime.WithLabelValues(project, ref).Set(r.TotalTime)
	c.totalCount.WithLabelValues(project, ref).Set(float64(r.TotalCount))
	c.successCount.WithLabelValues(project, ref).Set(float64(r.SuccessCount))
	c.failedCount.WithLabelValues(project, ref).Set(float64(r.FailedCount))
	c.skippedCount.WithLabelValues(project, ref).Set(float64(r.SkippedCount))
	c.errorCount.WithLabelValues(project, ref).Set(float64(r.ErrorCount))

	for _, suite := range r.TestSuites {
		sName := suite.Name

		c.suiteDuration.WithLabelValues(project, ref, sName).Set(suite.TotalTime)
		c.suiteCount.WithLabelValues(project, ref, sName).Set(float64(suite.TotalCount))

		// Individual test cases (optional, can be high-cardinality).
		if c.config.IncludeTestCases {
			for _, tc := range suite.TestCases {
				caseName := tc.Name
				c.caseDuration.WithLabelValues(project, ref, sName, caseName).Observe(tc.ExecutionTime)
				c.caseStatus.WithLabelValues(project, ref, sName, caseName, tc.Status).Set(1)
			}
		}
	}
}
