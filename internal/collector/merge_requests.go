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

// MergeRequestsCollector gathers merge-request analytics metrics.
type MergeRequestsCollector struct {
	client   *gitlabclient.Client
	config   config.MergeRequestsCollectorConfig
	projects []string
	mu       sync.RWMutex
	logger   *logrus.Entry

	// Prometheus descriptors
	timeToMerge       *prometheus.Desc
	timeToFirstReview *prometheus.Desc
	reviewCycles      *prometheus.Desc
	changesCount      *prometheus.Desc
	status            *prometheus.Desc
	throughput        *prometheus.Desc
	notesCount        *prometheus.Desc
	openDuration      *prometheus.Desc

	// Internal operational metrics
	scrapeDuration *prometheus.Desc
	scrapeErrors   *prometheus.Desc

	// Collected observations (mutex-protected)
	observations mergeRequestObservations
}

type mergeRequestObservations struct {
	timeToMerge       []labeledValue
	timeToFirstReview []labeledValue
	reviewCycles      []labeledValue
	changesCount      []labeledValue
	status            []labeledGauge
	throughput        []labeledGauge
	notesCount        []labeledValue
	openDuration      []labeledValue
	scrapeDuration    float64
	scrapeErrors      float64
}

type labeledValue struct {
	labels []string
	value  float64
}

type labeledGauge struct {
	labels []string
	value  float64
}

var defaultMRBuckets = []float64{60, 300, 600, 1800, 3600, 7200, 14400, 28800, 43200, 86400, 172800, 604800}

// NewMergeRequestsCollector creates a new MR analytics collector.
func NewMergeRequestsCollector(client *gitlabclient.Client, cfg config.MergeRequestsCollectorConfig, projects []string) *MergeRequestsCollector {
	buckets := cfg.HistogramBuckets
	if len(buckets) == 0 {
		buckets = defaultMRBuckets
	}
	_ = buckets // used below in Desc help text; actual bucket creation is at Collect time

	branchLabels := []string{"project", "target_branch"}
	statusLabels := []string{"project", "target_branch", "state"}

	return &MergeRequestsCollector{
		client:   client,
		config:   cfg,
		projects: projects,
		logger:   logrus.WithField("collector", "merge_requests"),

		timeToMerge: prometheus.NewDesc(
			"age_mr_time_to_merge_seconds",
			"Time from MR creation to merge in seconds.",
			branchLabels, nil,
		),
		timeToFirstReview: prometheus.NewDesc(
			"age_mr_time_to_first_review_seconds",
			"Time from MR creation to first review activity in seconds.",
			branchLabels, nil,
		),
		reviewCycles: prometheus.NewDesc(
			"age_mr_review_cycles_count",
			"Number of review cycles per merge request.",
			branchLabels, nil,
		),
		changesCount: prometheus.NewDesc(
			"age_mr_changes_count",
			"Number of changes (files changed) per merge request.",
			branchLabels, nil,
		),
		status: prometheus.NewDesc(
			"age_mr_status",
			"Current state of merge requests (1 = active).",
			statusLabels, nil,
		),
		throughput: prometheus.NewDesc(
			"age_mr_throughput_count",
			"Total number of merge requests merged.",
			branchLabels, nil,
		),
		notesCount: prometheus.NewDesc(
			"age_mr_notes_count",
			"Number of notes (comments) per merge request.",
			branchLabels, nil,
		),
		openDuration: prometheus.NewDesc(
			"age_mr_open_duration_seconds",
			"Duration a merge request has been or was open in seconds.",
			branchLabels, nil,
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

func (c *MergeRequestsCollector) Name() string  { return "merge_requests" }
func (c *MergeRequestsCollector) Enabled() bool { return c.config.Enabled }

// SetProjects updates the list of tracked projects.
func (c *MergeRequestsCollector) SetProjects(projects []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.projects = projects
}

// Describe implements prometheus.Collector.
func (c *MergeRequestsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.timeToMerge
	ch <- c.timeToFirstReview
	ch <- c.reviewCycles
	ch <- c.changesCount
	ch <- c.status
	ch <- c.throughput
	ch <- c.notesCount
	ch <- c.openDuration
	ch <- c.scrapeDuration
	ch <- c.scrapeErrors
}

// Collect implements prometheus.Collector.
func (c *MergeRequestsCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	obs := c.observations
	c.mu.RUnlock()

	buckets := c.config.HistogramBuckets
	if len(buckets) == 0 {
		buckets = defaultMRBuckets
	}

	emitHistograms(ch, c.timeToMerge, obs.timeToMerge, buckets)
	emitHistograms(ch, c.timeToFirstReview, obs.timeToFirstReview, buckets)
	emitHistograms(ch, c.reviewCycles, obs.reviewCycles, prometheus.DefBuckets)
	emitHistograms(ch, c.changesCount, obs.changesCount, prometheus.DefBuckets)
	emitHistograms(ch, c.notesCount, obs.notesCount, prometheus.DefBuckets)
	emitHistograms(ch, c.openDuration, obs.openDuration, buckets)

	for _, g := range obs.status {
		ch <- prometheus.MustNewConstMetric(c.status, prometheus.GaugeValue, g.value, g.labels...)
	}
	for _, g := range obs.throughput {
		ch <- prometheus.MustNewConstMetric(c.throughput, prometheus.CounterValue, g.value, g.labels...)
	}

	ch <- prometheus.MustNewConstMetric(c.scrapeDuration, prometheus.GaugeValue, obs.scrapeDuration, "merge_requests")
	ch <- prometheus.MustNewConstMetric(c.scrapeErrors, prometheus.CounterValue, obs.scrapeErrors, "merge_requests")
}

// Run performs one collection cycle.
func (c *MergeRequestsCollector) Run(ctx context.Context) error {
	start := time.Now()
	var errCount float64

	c.mu.RLock()
	projects := make([]string, len(c.projects))
	copy(projects, c.projects)
	c.mu.RUnlock()

	obs := mergeRequestObservations{}
	rest := c.client.REST()

	for _, project := range projects {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Fetch recently updated MRs (all states).
		opts := &gitlab.ListProjectMergeRequestsOptions{
			State:   gitlab.Ptr("all"),
			OrderBy: gitlab.Ptr("updated_at"),
			Sort:    gitlab.Ptr("desc"),
			ListOptions: gitlab.ListOptions{
				PerPage: 100,
				Page:    1,
			},
		}

		mrs, _, err := rest.MergeRequests.ListProjectMergeRequests(project, opts)
		if err != nil {
			c.logger.WithError(err).WithField("project", project).Error("failed to list merge requests")
			errCount++
			continue
		}

		throughputByBranch := make(map[string]float64)
		statusCounts := make(map[string]map[string]float64) // branch -> state -> count

		for _, mr := range mrs {
			targetBranch := mr.TargetBranch
			labels := []string{project, targetBranch}

			// Status tracking
			if _, ok := statusCounts[targetBranch]; !ok {
				statusCounts[targetBranch] = make(map[string]float64)
			}
			statusCounts[targetBranch][mr.State]++

			// Notes count
			obs.notesCount = append(obs.notesCount, labeledValue{
				labels: labels,
				value:  float64(mr.UserNotesCount),
			})

			// Changes count
			if mr.ChangesCount != "" {
				// ChangesCount is a string in the API; attempt rough parse.
				var changes float64
				for _, ch := range mr.ChangesCount {
					if ch >= '0' && ch <= '9' {
						changes = changes*10 + float64(ch-'0')
					}
				}
				obs.changesCount = append(obs.changesCount, labeledValue{
					labels: labels,
					value:  changes,
				})
			}

			now := time.Now()

			switch mr.State {
			case "merged":
				if mr.MergedAt != nil && mr.CreatedAt != nil {
					ttm := mr.MergedAt.Sub(*mr.CreatedAt).Seconds()
					obs.timeToMerge = append(obs.timeToMerge, labeledValue{
						labels: labels,
						value:  ttm,
					})
					obs.openDuration = append(obs.openDuration, labeledValue{
						labels: labels,
						value:  ttm,
					})
				}
				throughputByBranch[targetBranch]++

			case "closed":
				if mr.ClosedAt != nil && mr.CreatedAt != nil {
					dur := mr.ClosedAt.Sub(*mr.CreatedAt).Seconds()
					obs.openDuration = append(obs.openDuration, labeledValue{
						labels: labels,
						value:  dur,
					})
				}

			case "opened":
				if mr.CreatedAt != nil {
					dur := now.Sub(*mr.CreatedAt).Seconds()
					obs.openDuration = append(obs.openDuration, labeledValue{
						labels: labels,
						value:  dur,
					})
				}
			}

			// Approximate time to first review from UserNotesCount.
			// If there are notes, estimate based on MR age / note count.
			if mr.UserNotesCount > 0 && mr.CreatedAt != nil {
				var endTime time.Time
				if mr.MergedAt != nil {
					endTime = *mr.MergedAt
				} else if mr.ClosedAt != nil {
					endTime = *mr.ClosedAt
				} else {
					endTime = now
				}
				// Rough heuristic: first review ≈ total_duration / (notes + 1)
				totalDur := endTime.Sub(*mr.CreatedAt).Seconds()
				approxFirst := totalDur / float64(mr.UserNotesCount+1)
				obs.timeToFirstReview = append(obs.timeToFirstReview, labeledValue{
					labels: labels,
					value:  approxFirst,
				})
			}

			// Review cycles approximation: round-trips ≈ ceil(notes / 2).
			if mr.UserNotesCount > 0 {
				cycles := float64((mr.UserNotesCount + 1) / 2)
				obs.reviewCycles = append(obs.reviewCycles, labeledValue{
					labels: labels,
					value:  cycles,
				})
			}
		}

		// Emit status gauges.
		for branch, states := range statusCounts {
			for state, count := range states {
				obs.status = append(obs.status, labeledGauge{
					labels: []string{project, branch, state},
					value:  count,
				})
			}
		}

		// Emit throughput.
		for branch, count := range throughputByBranch {
			obs.throughput = append(obs.throughput, labeledGauge{
				labels: []string{project, branch},
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
	}).Debug("merge_requests collection completed")

	return nil
}

// emitHistograms emits constant histogram metrics for a set of observations.
func emitHistograms(ch chan<- prometheus.Metric, desc *prometheus.Desc, values []labeledValue, buckets []float64) {
	// Group by label set.
	type key struct{ a, b string }
	groups := make(map[key][]float64)
	groupLabels := make(map[key][]string)
	for _, v := range values {
		k := key{}
		if len(v.labels) > 0 {
			k.a = v.labels[0]
		}
		if len(v.labels) > 1 {
			k.b = v.labels[1]
		}
		groups[k] = append(groups[k], v.value)
		groupLabels[k] = v.labels
	}

	for k, vals := range groups {
		bucketCounts := make(map[float64]uint64)
		for _, b := range buckets {
			bucketCounts[b] = 0
		}
		var sum float64
		for _, v := range vals {
			sum += v
			for _, b := range buckets {
				if v <= b {
					bucketCounts[b]++
				}
			}
		}
		finalBuckets := make(map[float64]uint64)
		for _, b := range buckets {
			finalBuckets[b] = bucketCounts[b]
		}
		h, err := prometheus.NewConstHistogram(desc, uint64(len(vals)), sum, finalBuckets, groupLabels[k]...)
		if err != nil {
			logrus.WithError(err).WithField("key", k).Warn("failed to create histogram")
			continue
		}
		ch <- h
	}
}
