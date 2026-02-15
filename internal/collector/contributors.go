package collector

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/internal/config"
	gitlabclient "github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/internal/gitlab"
)

// ContributorsCollector gathers contributor analytics metrics.
type ContributorsCollector struct {
	client   *gitlabclient.Client
	config   config.ContributorsCollectorConfig
	projects []string
	mu       sync.RWMutex
	logger   *logrus.Entry

	// Prometheus descriptors
	commitsCount *prometheus.Desc
	additions    *prometheus.Desc
	deletions    *prometheus.Desc

	// Internal operational metrics
	scrapeDuration *prometheus.Desc
	scrapeErrors   *prometheus.Desc

	// Collected observations (mutex-protected)
	observations contributorsObservations
}

type contributorsObservations struct {
	commitsCount   []labeledGauge
	additions      []labeledGauge
	deletions      []labeledGauge
	scrapeDuration float64
	scrapeErrors   float64
}

// contributorResponse represents a contributor record from the GitLab Contributors API.
type contributorResponse struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Commits   int    `json:"commits"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// NewContributorsCollector creates a new contributor analytics collector.
func NewContributorsCollector(client *gitlabclient.Client, cfg config.ContributorsCollectorConfig, projects []string) *ContributorsCollector {
	authorLabels := []string{"project", "author"}

	return &ContributorsCollector{
		client:   client,
		config:   cfg,
		projects: projects,
		logger:   logrus.WithField("collector", "contributors"),

		commitsCount: prometheus.NewDesc(
			"age_contributor_commits_count",
			"Total number of commits by contributor.",
			authorLabels, nil,
		),
		additions: prometheus.NewDesc(
			"age_contributor_additions",
			"Total number of line additions by contributor.",
			authorLabels, nil,
		),
		deletions: prometheus.NewDesc(
			"age_contributor_deletions",
			"Total number of line deletions by contributor.",
			authorLabels, nil,
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

func (c *ContributorsCollector) Name() string  { return "contributors" }
func (c *ContributorsCollector) Enabled() bool { return c.config.Enabled }

// SetProjects updates the list of tracked projects.
func (c *ContributorsCollector) SetProjects(projects []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projects = projects
}

// Describe implements prometheus.Collector.
func (c *ContributorsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.commitsCount
	ch <- c.additions
	ch <- c.deletions
	ch <- c.scrapeDuration
	ch <- c.scrapeErrors
}

// Collect implements prometheus.Collector.
func (c *ContributorsCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	obs := c.observations
	c.mu.RUnlock()

	for _, g := range obs.commitsCount {
		ch <- prometheus.MustNewConstMetric(c.commitsCount, prometheus.GaugeValue, g.value, g.labels...)
	}
	for _, g := range obs.additions {
		ch <- prometheus.MustNewConstMetric(c.additions, prometheus.GaugeValue, g.value, g.labels...)
	}
	for _, g := range obs.deletions {
		ch <- prometheus.MustNewConstMetric(c.deletions, prometheus.GaugeValue, g.value, g.labels...)
	}

	ch <- prometheus.MustNewConstMetric(c.scrapeDuration, prometheus.GaugeValue, obs.scrapeDuration, "contributors")
	ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.CounterValue, obs.scrapeErrors, "contributors")
}

// Run performs one collection cycle.
func (c *ContributorsCollector) Run(ctx context.Context) error {
	start := time.Now()
	var errCount float64

	c.mu.RLock()
	projects := make([]string, len(c.projects))
	copy(projects, c.projects)
	c.mu.RUnlock()

	obs := contributorsObservations{}
	rest := c.client.REST()

	for _, project := range projects {
		if err := ctx.Err(); err != nil {
			return err
		}

		// GET /api/v4/projects/:id/repository/contributors
		path := fmt.Sprintf("projects/%s/repository/contributors", project)
		req, err := rest.NewRequest(http.MethodGet, path, nil, nil)
		if err != nil {
			c.logger.WithError(err).WithField("project", project).Error("failed to create contributors request")
			errCount++
			continue
		}

		var contributors []contributorResponse
		resp, err := rest.Do(req, &contributors)
		if err != nil {
			if resp != nil && resp.StatusCode == http.StatusNotFound {
				c.logger.WithField("project", project).Debug("contributors endpoint not found (empty repo?)")
				continue
			}
			c.logger.WithError(err).WithField("project", project).Error("failed to fetch contributors")
			errCount++
			continue
		}

		for _, contrib := range contributors {
			author := contrib.Name
			if author == "" {
				author = contrib.Email
			}
			labels := []string{project, author}

			obs.commitsCount = append(obs.commitsCount, labeledGauge{
				labels: labels,
				value:  float64(contrib.Commits),
			})
			obs.additions = append(obs.additions, labeledGauge{
				labels: labels,
				value:  float64(contrib.Additions),
			})
			obs.deletions = append(obs.deletions, labeledGauge{
				labels: labels,
				value:  float64(contrib.Deletions),
			})
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
	}).Debug("contributors collection completed")

	return nil
}
