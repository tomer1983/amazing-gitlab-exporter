// Package exporter wires together the GitLab client, collectors, scheduler,
// store, and HTTP server into a single orchestrator.
package exporter

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/internal/collector"
	"github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/internal/config"
	gitlabclient "github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/internal/gitlab"
	"github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/internal/scheduler"
	"github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/internal/server"
	"github.com/amazing-gitlab-exporter/amazing-gitlab-exporter/internal/store"
)

// Operational metrics.
var (
	projectsTracked = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "age_projects_tracked",
		Help: "Number of GitLab projects being monitored.",
	})
	gitlabTier = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "age_gitlab_tier",
		Help: "Detected GitLab tier (0=Free, 1=Premium, 2=Ultimate).",
	})
	collectorEnabled = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "age_collector_enabled",
		Help: "Whether a collector is enabled (1) or disabled (0).",
	}, []string{"collector_type"})
	apiRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "age_api_requests_total",
		Help: "Total GitLab API requests made.",
	}, []string{"method", "endpoint", "status_code"})
	apiRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "age_api_request_duration_seconds",
		Help:    "Duration of GitLab API requests.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "endpoint"})
)

func init() {
	prometheus.MustRegister(
		projectsTracked,
		gitlabTier,
		collectorEnabled,
		apiRequestsTotal,
		apiRequestDuration,
	)
}

// Exporter is the main application orchestrator.
type Exporter struct {
	config    *config.Config
	client    *gitlabclient.Client
	registry  *collector.Registry
	scheduler *scheduler.Scheduler
	server    *server.Server
	store     store.Store
	logger    *logrus.Entry
}

// NewExporter creates and initialises the exporter:
//  1. Creates the GitLab client.
//  2. Creates the store (Redis if configured, otherwise in-memory).
//  3. Runs tier detection.
//  4. Discovers projects.
//  5. Creates and registers collectors.
//  6. Creates the scheduler and HTTP server.
func NewExporter(cfg *config.Config, logger *logrus.Entry) (*Exporter, error) {
	log := logger.WithField("component", "exporter")

	// --- 1. GitLab client ---
	client, err := gitlabclient.New(
		cfg.GitLab.URL,
		cfg.GitLab.Token,
		cfg.GitLab.MaxRequestsPerSecond,
		cfg.GitLab.BurstRequestsPerSecond,
		cfg.GitLab.UseGraphQL,
		log,
	)
	if err != nil {
		return nil, fmt.Errorf("creating gitlab client: %w", err)
	}

	// --- 2. Store ---
	var st store.Store
	if cfg.Redis.URL != "" {
		rs, err := store.NewRedisStore(cfg.Redis.URL)
		if err != nil {
			return nil, fmt.Errorf("creating redis store: %w", err)
		}
		st = rs
		log.Info("using Redis store")
	} else {
		st = store.NewMemoryStore()
		log.Info("using in-memory store")
	}

	// --- 3. Tier detection ---
	features := client.Features()
	if features != nil {
		gitlabTier.Set(float64(features.Tier))
		log.WithField("tier", features.Tier).Info("gitlab tier detected")
	}

	// --- 4. Discover projects ---
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	projects, err := discoverProjects(ctx, cfg, client, log)
	if err != nil {
		return nil, fmt.Errorf("discovering projects: %w", err)
	}
	projectsTracked.Set(float64(len(projects)))
	log.WithField("count", len(projects)).Info("projects discovered")

	// --- 5. Create and register collectors ---
	registry := collector.NewRegistry(log)
	sched := scheduler.NewScheduler(log)

	registerCollectors(cfg, client, registry, sched, features, projects, log)

	// --- 6. HTTP server ---
	srv := server.NewServer(cfg, registry, log)

	return &Exporter{
		config:    cfg,
		client:    client,
		registry:  registry,
		scheduler: sched,
		server:    srv,
		store:     st,
		logger:    log,
	}, nil
}

// Run starts the scheduler and HTTP server, then blocks until ctx is
// cancelled. On cancellation it performs a graceful shutdown.
func (e *Exporter) Run(ctx context.Context) error {
	// Start scheduler.
	e.scheduler.Start(ctx)

	// Start HTTP server.
	if err := e.server.Start(ctx); err != nil {
		return fmt.Errorf("starting server: %w", err)
	}

	// Mark ready.
	e.server.SetReady(true)
	e.logger.Info("exporter is ready")

	// Block until context is cancelled.
	<-ctx.Done()

	e.logger.Info("shutting down exporter")

	// Graceful shutdown.
	e.server.SetReady(false)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := e.server.Stop(shutdownCtx); err != nil {
		e.logger.WithError(err).Error("error during server shutdown")
	}

	e.scheduler.Stop()

	if err := e.store.Close(); err != nil {
		e.logger.WithError(err).Error("error closing store")
	}

	return nil
}

// discoverProjects builds the list of project paths from the explicit list
// and from wildcard expansion via the GitLab API.
func discoverProjects(ctx context.Context, cfg *config.Config, client *gitlabclient.Client, logger *logrus.Entry) ([]string, error) {
	seen := make(map[string]struct{})
	var projects []string

	// Explicit projects.
	for _, p := range cfg.Projects {
		if _, ok := seen[p.Name]; !ok {
			seen[p.Name] = struct{}{}
			projects = append(projects, p.Name)
		}
	}

	// Wildcard expansion.
	for _, wc := range cfg.Wildcards {
		opts := &gitlab.ListProjectsOptions{
			Search:      gitlab.Ptr(wc.Search),
			Archived:    gitlab.Ptr(wc.Archived),
			ListOptions: gitlab.ListOptions{PerPage: 100, Page: 1},
		}
		for {
			projs, resp, err := client.REST().Projects.ListProjects(opts, gitlab.WithContext(ctx))
			if err != nil {
				logger.WithError(err).WithField("owner", wc.Owner.Name).
					Warn("failed to expand wildcard, skipping")
				break
			}
			for _, p := range projs {
				path := p.PathWithNamespace
				if _, ok := seen[path]; !ok {
					seen[path] = struct{}{}
					projects = append(projects, path)
				}
			}
			if resp.NextPage == 0 {
				break
			}
			opts.Page = resp.NextPage
		}
	}

	if len(projects) == 0 {
		return nil, fmt.Errorf("no projects configured or discovered")
	}

	return projects, nil
}

// registerCollectors creates all enabled collectors, registers them with the
// registry, and adds a scheduler task for each.
func registerCollectors(
	cfg *config.Config,
	client *gitlabclient.Client,
	registry *collector.Registry,
	sched *scheduler.Scheduler,
	features *gitlabclient.DetectedFeatures,
	projects []string,
	logger *logrus.Entry,
) {
	type collectorDef struct {
		name     string
		enabled  bool
		interval time.Duration
		create   func() collector.Collector
	}

	defs := []collectorDef{
		{
			name:     "pipelines",
			enabled:  cfg.Collectors.Pipelines.Enabled,
			interval: time.Duration(cfg.Collectors.Pipelines.IntervalSeconds) * time.Second,
			create: func() collector.Collector {
				return collector.NewPipelinesCollector(client, cfg.Collectors.Pipelines, projects)
			},
		},
		{
			name:     "jobs",
			enabled:  cfg.Collectors.Jobs.Enabled,
			interval: time.Duration(cfg.Collectors.Jobs.IntervalSeconds) * time.Second,
			create: func() collector.Collector {
				return collector.NewJobsCollector(client, cfg.Collectors.Jobs, projects)
			},
		},
		{
			name:     "merge_requests",
			enabled:  cfg.Collectors.MergeRequests.Enabled,
			interval: time.Duration(cfg.Collectors.MergeRequests.IntervalSeconds) * time.Second,
			create: func() collector.Collector {
				return collector.NewMergeRequestsCollector(client, cfg.Collectors.MergeRequests, projects)
			},
		},
		{
			name:     "environments",
			enabled:  cfg.Collectors.Environments.Enabled,
			interval: time.Duration(cfg.Collectors.Environments.IntervalSeconds) * time.Second,
			create: func() collector.Collector {
				return collector.NewEnvironmentsCollector(client, cfg.Collectors.Environments, projects)
			},
		},
		{
			name:     "test_reports",
			enabled:  cfg.Collectors.TestReports.Enabled,
			interval: time.Duration(cfg.Collectors.TestReports.IntervalSeconds) * time.Second,
			create: func() collector.Collector {
				return collector.NewTestReportsCollector(client, cfg.Collectors.TestReports, projects)
			},
		},
		{
			name:     "dora",
			enabled:  cfg.Collectors.DORA.Enabled && features != nil && features.HasDORA,
			interval: time.Duration(cfg.Collectors.DORA.IntervalSeconds) * time.Second,
			create: func() collector.Collector {
				return collector.NewDORACollector(client, cfg.Collectors.DORA, projects)
			},
		},
		{
			name:     "value_stream",
			enabled:  cfg.Collectors.ValueStream.Enabled && features != nil && features.HasValueStream,
			interval: time.Duration(cfg.Collectors.ValueStream.IntervalSeconds) * time.Second,
			create: func() collector.Collector {
				return collector.NewValueStreamCollector(client, cfg.Collectors.ValueStream, projects)
			},
		},
		{
			name:     "code_review",
			enabled:  cfg.Collectors.CodeReview.Enabled && features != nil && features.HasCodeReview,
			interval: time.Duration(cfg.Collectors.CodeReview.IntervalSeconds) * time.Second,
			create: func() collector.Collector {
				return collector.NewCodeReviewCollector(client, cfg.Collectors.CodeReview, projects)
			},
		},
		{
			name:     "repository",
			enabled:  cfg.Collectors.Repository.Enabled,
			interval: time.Duration(cfg.Collectors.Repository.IntervalSeconds) * time.Second,
			create: func() collector.Collector {
				return collector.NewRepositoryCollector(client, cfg.Collectors.Repository, projects)
			},
		},
		{
			name:     "contributors",
			enabled:  cfg.Collectors.Contributors.Enabled,
			interval: time.Duration(cfg.Collectors.Contributors.IntervalSeconds) * time.Second,
			create: func() collector.Collector {
				return collector.NewContributorsCollector(client, cfg.Collectors.Contributors, projects)
			},
		},
	}

	for _, d := range defs {
		if d.enabled {
			collectorEnabled.WithLabelValues(d.name).Set(1)
		} else {
			collectorEnabled.WithLabelValues(d.name).Set(0)
		}

		if !d.enabled {
			logger.WithField("collector", d.name).Info("collector disabled, skipping")
			continue
		}

		c := d.create()
		registry.Register(c)

		interval := d.interval
		if interval <= 0 {
			interval = 30 * time.Second
		}

		task := scheduler.NewTask(d.name, interval, c.Run, logger)
		sched.AddTask(task)

		logger.WithFields(logrus.Fields{
			"collector": d.name,
			"interval":  interval,
		}).Info("collector registered")
	}
}
