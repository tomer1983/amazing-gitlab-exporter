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

// DORACollector gathers DORA metrics from GitLab (Ultimate tier).
type DORACollector struct {
	client   *gitlabclient.Client
	config   config.DORACollectorConfig
	projects []string
	mu       sync.RWMutex
	logger   *logrus.Entry

	// Prometheus descriptors
	deploymentFrequency *prometheus.Desc
	leadTimeForChanges  *prometheus.Desc
	timeToRestore       *prometheus.Desc
	changeFailureRate   *prometheus.Desc

	// Internal operational metrics
	scrapeDuration *prometheus.Desc
	scrapeErrors   *prometheus.Desc

	// Collected observations (mutex-protected)
	observations doraObservations
}

type doraObservations struct {
	deploymentFrequency []labeledGauge
	leadTimeForChanges  []labeledGauge
	timeToRestore       []labeledGauge
	changeFailureRate   []labeledGauge
	scrapeDuration      float64
	scrapeErrors        float64
}

// doraAPIResponse represents a single DORA metric data point from the GitLab API.
type doraAPIResponse struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// NewDORACollector creates a new DORA metrics collector.
func NewDORACollector(client *gitlabclient.Client, cfg config.DORACollectorConfig, projects []string) *DORACollector {
	envLabels := []string{"project", "environment_tier"}

	return &DORACollector{
		client:   client,
		config:   cfg,
		projects: projects,
		logger:   logrus.WithField("collector", "dora"),

		deploymentFrequency: prometheus.NewDesc(
			"age_dora_deployment_frequency",
			"Number of deployments per day.",
			envLabels, nil,
		),
		leadTimeForChanges: prometheus.NewDesc(
			"age_dora_lead_time_for_changes_seconds",
			"Median time from commit to deploy in seconds.",
			envLabels, nil,
		),
		timeToRestore: prometheus.NewDesc(
			"age_dora_time_to_restore_service_seconds",
			"Median time to restore service in seconds.",
			envLabels, nil,
		),
		changeFailureRate: prometheus.NewDesc(
			"age_dora_change_failure_rate",
			"Percentage of deployments causing failures (0-100).",
			envLabels, nil,
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

func (c *DORACollector) Name() string  { return "dora" }
func (c *DORACollector) Enabled() bool { return c.config.Enabled }

// SetProjects updates the list of tracked projects.
func (c *DORACollector) SetProjects(projects []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projects = projects
}

// Describe implements prometheus.Collector.
func (c *DORACollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.deploymentFrequency
	ch <- c.leadTimeForChanges
	ch <- c.timeToRestore
	ch <- c.changeFailureRate
	ch <- c.scrapeDuration
	ch <- c.scrapeErrors
}

// Collect implements prometheus.Collector.
func (c *DORACollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	obs := c.observations
	c.mu.RUnlock()

	for _, g := range obs.deploymentFrequency {
		ch <- prometheus.MustNewConstMetric(c.deploymentFrequency, prometheus.GaugeValue, g.value, g.labels...)
	}
	for _, g := range obs.leadTimeForChanges {
		ch <- prometheus.MustNewConstMetric(c.leadTimeForChanges, prometheus.GaugeValue, g.value, g.labels...)
	}
	for _, g := range obs.timeToRestore {
		ch <- prometheus.MustNewConstMetric(c.timeToRestore, prometheus.GaugeValue, g.value, g.labels...)
	}
	for _, g := range obs.changeFailureRate {
		ch <- prometheus.MustNewConstMetric(c.changeFailureRate, prometheus.GaugeValue, g.value, g.labels...)
	}

	ch <- prometheus.MustNewConstMetric(c.scrapeDuration, prometheus.GaugeValue, obs.scrapeDuration, "dora")
	ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.CounterValue, obs.scrapeErrors, "dora")
}

// Run performs one collection cycle.
func (c *DORACollector) Run(ctx context.Context) error {
	start := time.Now()
	var errCount float64

	// Gate on tier feature availability.
	if features := c.client.Features(); features == nil || !features.HasDORA {
		c.logger.Debug("DORA metrics not available on this GitLab instance, skipping")
		c.mu.Lock()
		c.observations = doraObservations{
			scrapeDuration: time.Since(start).Seconds(),
		}
		c.mu.Unlock()
		return nil
	}

	c.mu.RLock()
	projects := make([]string, len(c.projects))
	copy(projects, c.projects)
	c.mu.RUnlock()

	envTiers := c.config.EnvironmentTiers
	if len(envTiers) == 0 {
		envTiers = []string{"production"}
	}

	obs := doraObservations{}
	rest := c.client.REST()

	doraMetricTypes := []string{
		"deployment_frequency",
		"lead_time_for_changes",
		"time_to_restore_service",
		"change_failure_rate",
	}

	for _, project := range projects {
		if err := ctx.Err(); err != nil {
			return err
		}

		for _, envTier := range envTiers {
			for _, metricType := range doraMetricTypes {
				labels := []string{project, envTier}

				path := fmt.Sprintf("projects/%s/dora/metrics", project)

				type doraOpt struct {
					Metric          string `url:"metric"`
					EnvironmentTier string `url:"environment_tier,omitempty"`
					Interval        string `url:"interval,omitempty"`
				}

				reqOpt := &doraOpt{
					Metric:          metricType,
					EnvironmentTier: envTier,
					Interval:        "daily",
				}

				req, err := rest.NewRequest(http.MethodGet, path, reqOpt, nil)
				if err != nil {
					c.logger.WithError(err).WithFields(logrus.Fields{
						"project": project,
						"metric":  metricType,
						"tier":    envTier,
					}).Error("failed to create DORA request")
					errCount++
					continue
				}

				var results []doraAPIResponse
				resp, err := rest.Do(req, &results)
				if err != nil {
					if resp != nil && resp.StatusCode == http.StatusForbidden {
						c.logger.WithFields(logrus.Fields{
							"project": project,
							"metric":  metricType,
						}).Debug("DORA metric not accessible for project")
						continue
					}
					c.logger.WithError(err).WithFields(logrus.Fields{
						"project": project,
						"metric":  metricType,
						"tier":    envTier,
					}).Error("failed to fetch DORA metric")
					errCount++
					continue
				}

				// Use the most recent data point.
				if len(results) == 0 {
					continue
				}
				latestValue := results[len(results)-1].Value

				switch metricType {
				case "deployment_frequency":
					obs.deploymentFrequency = append(obs.deploymentFrequency, labeledGauge{
						labels: labels,
						value:  latestValue,
					})
				case "lead_time_for_changes":
					// Value from API is in seconds.
					obs.leadTimeForChanges = append(obs.leadTimeForChanges, labeledGauge{
						labels: labels,
						value:  latestValue,
					})
				case "time_to_restore_service":
					// Value from API is in seconds.
					obs.timeToRestore = append(obs.timeToRestore, labeledGauge{
						labels: labels,
						value:  latestValue,
					})
				case "change_failure_rate":
					// Value from API is a rate (percentage).
					obs.changeFailureRate = append(obs.changeFailureRate, labeledGauge{
						labels: labels,
						value:  latestValue,
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
	}).Debug("dora collection completed")

	return nil
}
