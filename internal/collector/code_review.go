package collector

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/internal/config"
	gitlabclient "github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/internal/gitlab"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// CodeReviewCollector gathers code review analytics metrics (Premium tier).
type CodeReviewCollector struct {
	client   *gitlabclient.Client
	config   config.CodeReviewCollectorConfig
	projects []string
	mu       sync.RWMutex
	logger   *logrus.Entry

	// Prometheus descriptors
	turnaround     *prometheus.Desc
	approvalCount  *prometheus.Desc
	pendingCount   *prometheus.Desc
	requestedCount *prometheus.Desc

	// Internal operational metrics
	scrapeDuration *prometheus.Desc
	scrapeErrors   *prometheus.Desc

	// Collected observations (mutex-protected)
	observations codeReviewObservations
}

type codeReviewObservations struct {
	turnaround     []labeledValue
	approvalCount  []labeledGauge
	pendingCount   []labeledGauge
	requestedCount []labeledGauge
	scrapeDuration float64
	scrapeErrors   float64
}

var defaultReviewBuckets = []float64{300, 600, 1800, 3600, 7200, 14400, 28800, 43200, 86400, 172800, 604800}

// NewCodeReviewCollector creates a new code review analytics collector.
func NewCodeReviewCollector(client *gitlabclient.Client, cfg config.CodeReviewCollectorConfig, projects []string) *CodeReviewCollector {
	return &CodeReviewCollector{
		client:   client,
		config:   cfg,
		projects: projects,
		logger:   logrus.WithField("collector", "code_review"),

		turnaround: prometheus.NewDesc(
			"age_review_turnaround_seconds",
			"Time taken for a reviewer to provide a review in seconds.",
			[]string{"project", "reviewer"}, nil,
		),
		approvalCount: prometheus.NewDesc(
			"age_review_approval_count",
			"Total number of approvals by reviewer.",
			[]string{"project", "reviewer"}, nil,
		),
		pendingCount: prometheus.NewDesc(
			"age_review_pending_count",
			"Number of merge requests awaiting review.",
			[]string{"project"}, nil,
		),
		requestedCount: prometheus.NewDesc(
			"age_review_requested_count",
			"Total number of review requests received by reviewer.",
			[]string{"project", "reviewer"}, nil,
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

func (c *CodeReviewCollector) Name() string  { return "code_review" }
func (c *CodeReviewCollector) Enabled() bool { return c.config.Enabled }

// SetProjects updates the list of tracked projects.
func (c *CodeReviewCollector) SetProjects(projects []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projects = projects
}

// Describe implements prometheus.Collector.
func (c *CodeReviewCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.turnaround
	ch <- c.approvalCount
	ch <- c.pendingCount
	ch <- c.requestedCount
	ch <- c.scrapeDuration
	ch <- c.scrapeErrors
}

// Collect implements prometheus.Collector.
func (c *CodeReviewCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	obs := c.observations
	c.mu.RUnlock()

	emitHistograms(ch, c.turnaround, obs.turnaround, defaultReviewBuckets)

	for _, g := range obs.approvalCount {
		ch <- prometheus.MustNewConstMetric(c.approvalCount, prometheus.CounterValue, g.value, g.labels...)
	}
	for _, g := range obs.pendingCount {
		ch <- prometheus.MustNewConstMetric(c.pendingCount, prometheus.GaugeValue, g.value, g.labels...)
	}
	for _, g := range obs.requestedCount {
		ch <- prometheus.MustNewConstMetric(c.requestedCount, prometheus.CounterValue, g.value, g.labels...)
	}

	ch <- prometheus.MustNewConstMetric(c.scrapeDuration, prometheus.GaugeValue, obs.scrapeDuration, "code_review")
	ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.CounterValue, obs.scrapeErrors, "code_review")
}

// Run performs one collection cycle.
func (c *CodeReviewCollector) Run(ctx context.Context) error {
	start := time.Now()
	var errCount float64

	// Gate on tier feature availability.
	if features := c.client.Features(); features == nil || !features.HasCodeReview {
		c.logger.Debug("code review analytics not available on this GitLab instance, skipping")
		c.mu.Lock()
		c.observations = codeReviewObservations{
			scrapeDuration: time.Since(start).Seconds(),
		}
		c.mu.Unlock()
		return nil
	}

	c.mu.RLock()
	projects := make([]string, len(c.projects))
	copy(projects, c.projects)
	c.mu.RUnlock()

	obs := codeReviewObservations{}
	rest := c.client.REST()

	for _, project := range projects {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Fetch open MRs with reviewer information to determine pending reviews and reviewer stats.
		opts := &gitlab.ListProjectMergeRequestsOptions{
			State:   gitlab.Ptr("opened"),
			OrderBy: gitlab.Ptr("updated_at"),
			Sort:    gitlab.Ptr("desc"),
			ListOptions: gitlab.ListOptions{
				PerPage: 100,
				Page:    1,
			},
		}

		mrs, _, err := rest.MergeRequests.ListProjectMergeRequests(project, opts)
		if err != nil {
			c.logger.WithError(err).WithField("project", project).Error("failed to list MRs for code review")
			errCount++
			continue
		}

		var pendingReviews float64
		reviewerRequested := make(map[string]float64)
		reviewerApprovals := make(map[string]float64)

		for _, mr := range mrs {
			// Count pending reviews (opened MRs with reviewers assigned).
			if len(mr.Reviewers) > 0 {
				pendingReviews++
			}

			for _, reviewer := range mr.Reviewers {
				reviewerName := reviewer.Username
				reviewerRequested[reviewerName]++
			}

			// Approximate turnaround: if MR has notes, estimate time to first review.
			if mr.UserNotesCount > 0 && mr.CreatedAt != nil {
				now := time.Now()
				totalDur := now.Sub(*mr.CreatedAt).Seconds()
				approxTurnaround := totalDur / float64(mr.UserNotesCount+1)
				// We don't have per-reviewer info here; use first reviewer if available.
				if len(mr.Reviewers) > 0 {
					obs.turnaround = append(obs.turnaround, labeledValue{
						labels: []string{project, mr.Reviewers[0].Username},
						value:  approxTurnaround,
					})
				}
			}
		}

		// Also fetch recently merged MRs for approval stats.
		mergedOpts := &gitlab.ListProjectMergeRequestsOptions{
			State:   gitlab.Ptr("merged"),
			OrderBy: gitlab.Ptr("updated_at"),
			Sort:    gitlab.Ptr("desc"),
			ListOptions: gitlab.ListOptions{
				PerPage: 100,
				Page:    1,
			},
		}

		mergedMRs, _, err := rest.MergeRequests.ListProjectMergeRequests(project, mergedOpts)
		if err != nil {
			c.logger.WithError(err).WithField("project", project).Error("failed to list merged MRs for code review")
			errCount++
		} else {
			for _, mr := range mergedMRs {
				// Count approvals from merge info.
				if mr.MergedBy != nil {
					reviewerApprovals[mr.MergedBy.Username]++
				}

				// Turnaround for merged MRs.
				if mr.MergedAt != nil && mr.CreatedAt != nil && mr.UserNotesCount > 0 {
					totalDur := mr.MergedAt.Sub(*mr.CreatedAt).Seconds()
					approxTurnaround := totalDur / float64(mr.UserNotesCount+1)
					if len(mr.Reviewers) > 0 {
						obs.turnaround = append(obs.turnaround, labeledValue{
							labels: []string{project, mr.Reviewers[0].Username},
							value:  approxTurnaround,
						})
					}
				}

				for _, reviewer := range mr.Reviewers {
					reviewerRequested[reviewer.Username]++
				}
			}
		}

		obs.pendingCount = append(obs.pendingCount, labeledGauge{
			labels: []string{project},
			value:  pendingReviews,
		})

		for reviewer, count := range reviewerApprovals {
			obs.approvalCount = append(obs.approvalCount, labeledGauge{
				labels: []string{project, reviewer},
				value:  count,
			})
		}

		for reviewer, count := range reviewerRequested {
			obs.requestedCount = append(obs.requestedCount, labeledGauge{
				labels: []string{project, reviewer},
				value:  count,
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
	}).Debug("code_review collection completed")

	return nil
}
