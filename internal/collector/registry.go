// Package collector provides Prometheus metric collectors for GitLab data.
package collector

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

// Collector is the interface all metric collectors implement.
type Collector interface {
	// Name returns the human-readable name of the collector (e.g. "pipelines").
	Name() string
	// Enabled reports whether this collector is active per configuration.
	Enabled() bool
	// Describe sends the super-set of all possible metric descriptors.
	Describe(ch chan<- *prometheus.Desc)
	// Collect sends the current metric values.
	Collect(ch chan<- prometheus.Metric)
	// Run fetches data from GitLab and updates internal metric state for all
	// tracked projects. It should be called periodically by the scheduler.
	Run(ctx context.Context) error
	// SetProjects updates the set of project paths to track.
	SetProjects(projects []string)
}

// Registry holds all registered collectors and implements the
// prometheus.Collector interface so it can be registered with a
// prometheus.Registry directly.
type Registry struct {
	collectors []Collector
	mu         sync.RWMutex
	logger     *logrus.Entry
}

// compile-time check
var _ prometheus.Collector = (*Registry)(nil)

// NewRegistry creates an empty Registry.
func NewRegistry(logger *logrus.Entry) *Registry {
	return &Registry{
		logger: logger,
	}
}

// Register adds a collector to the registry.
func (r *Registry) Register(c Collector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collectors = append(r.collectors, c)
	r.logger.WithFields(logrus.Fields{
		"collector": c.Name(),
		"enabled":   c.Enabled(),
	}).Info("registered collector")
}

// Collectors returns a snapshot of all registered collectors.
func (r *Registry) Collectors() []Collector {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Collector, len(r.collectors))
	copy(out, r.collectors)
	return out
}

// Describe implements prometheus.Collector. It sends the descriptor super-set
// from every registered collector (enabled or not, per Prometheus conventions).
func (r *Registry) Describe(ch chan<- *prometheus.Desc) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, c := range r.collectors {
		c.Describe(ch)
	}
}

// Collect implements prometheus.Collector. It sends metrics only from enabled
// collectors.
func (r *Registry) Collect(ch chan<- prometheus.Metric) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, c := range r.collectors {
		if c.Enabled() {
			c.Collect(ch)
		}
	}
}

// RunAll runs the Run method of every enabled collector sequentially,
// returning the first error encountered.
func (r *Registry) RunAll(ctx context.Context) error {
	r.mu.RLock()
	collectors := make([]Collector, len(r.collectors))
	copy(collectors, r.collectors)
	r.mu.RUnlock()

	for _, c := range collectors {
		if !c.Enabled() {
			continue
		}
		r.logger.WithField("collector", c.Name()).Debug("running collector")
		if err := c.Run(ctx); err != nil {
			r.logger.WithFields(logrus.Fields{
				"collector": c.Name(),
				"error":     err,
			}).Error("collector run failed")
			return err
		}
	}
	return nil
}
