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

// ValueStreamCollector gathers Value Stream Analytics metrics (Premium tier).
type ValueStreamCollector struct {
	client   *gitlabclient.Client
	config   config.ValueStreamCollectorConfig
	projects []string
	mu       sync.RWMutex
	logger   *logrus.Entry

	// Prometheus descriptors
	stageDuration *prometheus.Desc
	cycleTime     *prometheus.Desc
	leadTime      *prometheus.Desc

	// Internal operational metrics
	scrapeDuration *prometheus.Desc
	scrapeErrors   *prometheus.Desc

	// Collected observations (mutex-protected)
	observations valueStreamObservations
}

type valueStreamObservations struct {
	stageDuration  []labeledGauge
	cycleTime      []labeledGauge
	leadTime       []labeledGauge
	scrapeDuration float64
	scrapeErrors   float64
}

// vsaStageResponse represents a Value Stream Analytics stage from the API.
type vsaStageResponse struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Name  string `json:"name"`
}

// vsaStageMedianResponse holds the median duration data for a stage.
type vsaStageMedianResponse struct {
	ID    int     `json:"id"`
	Value float64 `json:"value"` // seconds
}

// vsaSummaryResponse represents a Value Stream summary record.
type vsaSummaryResponse struct {
	Value      float64 `json:"value"`
	Unit       string  `json:"unit,omitempty"`
	Identifier string  `json:"identifier,omitempty"`
}

// vsaValueStreamResponse represents a value stream object.
type vsaValueStreamResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// NewValueStreamCollector creates a new Value Stream Analytics collector.
func NewValueStreamCollector(client *gitlabclient.Client, cfg config.ValueStreamCollectorConfig, projects []string) *ValueStreamCollector {
	stageLabels := []string{"project", "stage_name"}
	projectLabels := []string{"project"}

	return &ValueStreamCollector{
		client:   client,
		config:   cfg,
		projects: projects,
		logger:   logrus.WithField("collector", "value_stream"),

		stageDuration: prometheus.NewDesc(
			"age_value_stream_stage_duration_seconds",
			"Median time spent in a Value Stream Analytics stage in seconds.",
			stageLabels, nil,
		),
		cycleTime: prometheus.NewDesc(
			"age_value_stream_cycle_time_seconds",
			"Total cycle time across all stages in seconds.",
			projectLabels, nil,
		),
		leadTime: prometheus.NewDesc(
			"age_value_stream_lead_time_seconds",
			"Total lead time from issue to production in seconds.",
			projectLabels, nil,
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

func (c *ValueStreamCollector) Name() string  { return "value_stream" }
func (c *ValueStreamCollector) Enabled() bool { return c.config.Enabled }

// SetProjects updates the list of tracked projects.
func (c *ValueStreamCollector) SetProjects(projects []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projects = projects
}

// Describe implements prometheus.Collector.
func (c *ValueStreamCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.stageDuration
	ch <- c.cycleTime
	ch <- c.leadTime
	ch <- c.scrapeDuration
	ch <- c.scrapeErrors
}

// Collect implements prometheus.Collector.
func (c *ValueStreamCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	obs := c.observations
	c.mu.RUnlock()

	for _, g := range obs.stageDuration {
		ch <- prometheus.MustNewConstMetric(c.stageDuration, prometheus.GaugeValue, g.value, g.labels...)
	}
	for _, g := range obs.cycleTime {
		ch <- prometheus.MustNewConstMetric(c.cycleTime, prometheus.GaugeValue, g.value, g.labels...)
	}
	for _, g := range obs.leadTime {
		ch <- prometheus.MustNewConstMetric(c.leadTime, prometheus.GaugeValue, g.value, g.labels...)
	}

	ch <- prometheus.MustNewConstMetric(c.scrapeDuration, prometheus.GaugeValue, obs.scrapeDuration, "value_stream")
	ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.CounterValue, obs.scrapeErrors, "value_stream")
}

// Run performs one collection cycle.
func (c *ValueStreamCollector) Run(ctx context.Context) error {
	start := time.Now()
	var errCount float64

	// Gate on tier feature availability.
	if features := c.client.Features(); features == nil || !features.HasValueStream {
		c.logger.Debug("value stream analytics not available on this GitLab instance, skipping")
		c.mu.Lock()
		c.observations = valueStreamObservations{
			scrapeDuration: time.Since(start).Seconds(),
		}
		c.mu.Unlock()
		return nil
	}

	c.mu.RLock()
	projects := make([]string, len(c.projects))
	copy(projects, c.projects)
	c.mu.RUnlock()

	obs := valueStreamObservations{}
	rest := c.client.REST()

	for _, project := range projects {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Step 1: List value streams for the project.
		vsPath := fmt.Sprintf("projects/%s/analytics/value_stream_analytics/value_streams", project)
		req, err := rest.NewRequest(http.MethodGet, vsPath, nil, nil)
		if err != nil {
			c.logger.WithError(err).WithField("project", project).Error("failed to create value streams request")
			errCount++
			continue
		}

		var valueStreams []vsaValueStreamResponse
		resp, err := rest.Do(req, &valueStreams)
		if err != nil {
			if resp != nil && (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound) {
				c.logger.WithField("project", project).Debug("value stream analytics not accessible for project")
				continue
			}
			c.logger.WithError(err).WithField("project", project).Error("failed to list value streams")
			errCount++
			continue
		}

		if len(valueStreams) == 0 {
			continue
		}

		// Use the first (default) value stream.
		vsID := valueStreams[0].ID

		// Step 2: List stages for this value stream.
		stagesPath := fmt.Sprintf("projects/%s/analytics/value_stream_analytics/value_streams/%d/stages", project, vsID)
		req, err = rest.NewRequest(http.MethodGet, stagesPath, nil, nil)
		if err != nil {
			c.logger.WithError(err).WithField("project", project).Error("failed to create stages request")
			errCount++
			continue
		}

		var stages []vsaStageResponse
		_, err = rest.Do(req, &stages)
		if err != nil {
			c.logger.WithError(err).WithField("project", project).Error("failed to list VSA stages")
			errCount++
			continue
		}

		var totalCycleTime float64

		// Step 3: Get median duration for each stage.
		for _, stage := range stages {
			stageName := stage.Name
			if stageName == "" {
				stageName = stage.Title
			}

			medianPath := fmt.Sprintf(
				"projects/%s/analytics/value_stream_analytics/value_streams/%d/stages/%d/median",
				project, vsID, stage.ID,
			)
			req, err = rest.NewRequest(http.MethodGet, medianPath, nil, nil)
			if err != nil {
				c.logger.WithError(err).WithFields(logrus.Fields{
					"project": project,
					"stage":   stageName,
				}).Error("failed to create stage median request")
				errCount++
				continue
			}

			var median vsaStageMedianResponse
			_, err = rest.Do(req, &median)
			if err != nil {
				c.logger.WithError(err).WithFields(logrus.Fields{
					"project": project,
					"stage":   stageName,
				}).Warn("failed to fetch stage median")
				continue
			}

			durationSec := median.Value
			obs.stageDuration = append(obs.stageDuration, labeledGauge{
				labels: []string{project, stageName},
				value:  durationSec,
			})
			totalCycleTime += durationSec
		}

		obs.cycleTime = append(obs.cycleTime, labeledGauge{
			labels: []string{project},
			value:  totalCycleTime,
		})

		// Lead time ≈ cycle time for the default value stream (issue → production).
		obs.leadTime = append(obs.leadTime, labeledGauge{
			labels: []string{project},
			value:  totalCycleTime,
		})
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
	}).Debug("value_stream collection completed")

	return nil
}
