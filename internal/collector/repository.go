package collector

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/internal/config"
	gitlabclient "github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/internal/gitlab"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// RepositoryCollector gathers repository analytics metrics.
type RepositoryCollector struct {
	client   *gitlabclient.Client
	config   config.RepositoryCollectorConfig
	projects []string
	mu       sync.RWMutex
	logger   *logrus.Entry

	// Prometheus descriptors
	languagePercentage *prometheus.Desc
	commitCount        *prometheus.Desc
	sizeBytes          *prometheus.Desc
	coverage           *prometheus.Desc

	// Internal operational metrics
	scrapeDuration *prometheus.Desc
	scrapeErrors   *prometheus.Desc

	// Collected observations (mutex-protected)
	observations repositoryObservations
}

type repositoryObservations struct {
	languagePercentage []labeledGauge
	commitCount        []labeledGauge
	sizeBytes          []labeledGauge
	coverage           []labeledGauge
	scrapeDuration     float64
	scrapeErrors       float64
}

// NewRepositoryCollector creates a new repository analytics collector.
func NewRepositoryCollector(client *gitlabclient.Client, cfg config.RepositoryCollectorConfig, projects []string) *RepositoryCollector {
	return &RepositoryCollector{
		client:   client,
		config:   cfg,
		projects: projects,
		logger:   logrus.WithField("collector", "repository"),

		languagePercentage: prometheus.NewDesc(
			"age_repository_language_percentage",
			"Percentage of repository code in a given language.",
			[]string{"project", "language"}, nil,
		),
		commitCount: prometheus.NewDesc(
			"age_repository_commit_count",
			"Total number of commits in the repository.",
			[]string{"project", "ref"}, nil,
		),
		sizeBytes: prometheus.NewDesc(
			"age_repository_size_bytes",
			"Total repository size in bytes.",
			[]string{"project"}, nil,
		),
		coverage: prometheus.NewDesc(
			"age_repository_coverage",
			"Latest test coverage percentage for the project.",
			[]string{"project"}, nil,
		),

		scrapeDuration: prometheus.NewDesc(
			"age_scrape_duration_seconds",
			"Time taken by the collector scrape.",
			[]string{"collector_type"}, nil,
		),
		scrapeErrors: prometheus.NewDesc(
			"age_scrape_errors_total",
			"Total number of scrape errors.",
			[]string{"collector_type"}, nil,
		),
	}
}

func (c *RepositoryCollector) Name() string  { return "repository" }
func (c *RepositoryCollector) Enabled() bool { return c.config.Enabled }

// SetProjects updates the list of tracked projects.
func (c *RepositoryCollector) SetProjects(projects []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projects = projects
}

// Describe implements prometheus.Collector.
func (c *RepositoryCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.languagePercentage
	ch <- c.commitCount
	ch <- c.sizeBytes
	ch <- c.coverage
	ch <- c.scrapeDuration
	ch <- c.scrapeErrors
}

// Collect implements prometheus.Collector.
func (c *RepositoryCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	obs := c.observations
	c.mu.RUnlock()

	for _, g := range obs.languagePercentage {
		ch <- prometheus.MustNewConstMetric(c.languagePercentage, prometheus.GaugeValue, g.value, g.labels...)
	}
	for _, g := range obs.commitCount {
		ch <- prometheus.MustNewConstMetric(c.commitCount, prometheus.GaugeValue, g.value, g.labels...)
	}
	for _, g := range obs.sizeBytes {
		ch <- prometheus.MustNewConstMetric(c.sizeBytes, prometheus.GaugeValue, g.value, g.labels...)
	}
	for _, g := range obs.coverage {
		ch <- prometheus.MustNewConstMetric(c.coverage, prometheus.GaugeValue, g.value, g.labels...)
	}

	ch <- prometheus.MustNewConstMetric(c.scrapeDuration, prometheus.GaugeValue, obs.scrapeDuration, "repository")
	ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.CounterValue, obs.scrapeErrors, "repository")
}

// Run performs one collection cycle.
func (c *RepositoryCollector) Run(ctx context.Context) error {
	start := time.Now()
	var errCount float64

	c.mu.RLock()
	projects := make([]string, len(c.projects))
	copy(projects, c.projects)
	c.mu.RUnlock()

	obs := repositoryObservations{}
	rest := c.client.REST()

	for _, project := range projects {
		if err := ctx.Err(); err != nil {
			return err
		}

		// --- Languages ---
		languages, _, err := rest.Projects.GetProjectLanguages(project)
		if err != nil {
			c.logger.WithError(err).WithField("project", project).Error("failed to get repository languages")
			errCount++
		} else if languages != nil {
			for lang, pct := range *languages {
				obs.languagePercentage = append(obs.languagePercentage, labeledGauge{
					labels: []string{project, lang},
					value:  float64(pct),
				})
			}
		}

		// --- Project statistics (size, commit count) ---
		projDetail, _, err := rest.Projects.GetProject(project, &gitlab.GetProjectOptions{
			Statistics: gitlab.Ptr(true),
		})
		if err != nil {
			c.logger.WithError(err).WithField("project", project).Error("failed to get project statistics")
			errCount++
		} else if projDetail != nil {
			if projDetail.Statistics != nil {
				obs.sizeBytes = append(obs.sizeBytes, labeledGauge{
					labels: []string{project},
					value:  float64(projDetail.Statistics.RepositorySize),
				})
				obs.commitCount = append(obs.commitCount, labeledGauge{
					labels: []string{project, projDetail.DefaultBranch},
					value:  float64(projDetail.Statistics.CommitCount),
				})
			}
		}

		// --- Coverage from latest pipeline ---
		pipelines, _, err := rest.Pipelines.ListProjectPipelines(project, &gitlab.ListProjectPipelinesOptions{
			Scope:   gitlab.Ptr("finished"),
			OrderBy: gitlab.Ptr("updated_at"),
			Sort:    gitlab.Ptr("desc"),
			ListOptions: gitlab.ListOptions{
				PerPage: 1,
				Page:    1,
			},
		})
		if err != nil {
			c.logger.WithError(err).WithField("project", project).Warn("failed to get latest pipeline for coverage")
		} else if len(pipelines) > 0 {
			// PipelineInfo doesn't have Coverage — fetch the full Pipeline.
			if fullPipeline, _, err2 := rest.Pipelines.GetPipeline(project, pipelines[0].ID); err2 == nil && fullPipeline.Coverage != "" {
				if cov, parseErr := strconv.ParseFloat(fullPipeline.Coverage, 64); parseErr == nil {
					obs.coverage = append(obs.coverage, labeledGauge{
						labels: []string{project},
						value:  cov,
					})
				}
			}
		}
	}

	obs.scrapeDuration = time.Since(start).Seconds()
	obs.scrapeErrors = errCount

	c.mu.Lock()
	c.observations = obs
	c.mu.Unlock()

	c.logger.WithFields(logrus.Fields{
		"duration": obs.scrapeDuration,
		"errors":   errCount,
		"projects": len(projects),
	}).Debug("repository collection completed")

	return nil
}
