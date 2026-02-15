package scheduler

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

// Task represents a periodically executed unit of work (typically a collector's Run method).
type Task struct {
	// Name is a human-readable identifier used in log messages.
	Name string
	// Interval is the period between successive runs.
	Interval time.Duration
	// RunFunc is the function executed each tick. Errors are logged but do not
	// stop the loop.
	RunFunc func(ctx context.Context) error
	logger  *logrus.Entry
}

// NewTask creates a new periodic task.
func NewTask(name string, interval time.Duration, runFunc func(ctx context.Context) error, logger *logrus.Entry) *Task {
	return &Task{
		Name:     name,
		Interval: interval,
		RunFunc:  runFunc,
		logger:   logger.WithField("task", name),
	}
}

// Run executes the task in a loop. It fires immediately on entry, then waits
// for Interval between subsequent invocations. The loop exits when ctx is done.
func (t *Task) Run(ctx context.Context) {
	t.logger.WithField("interval", t.Interval).Info("task started")

	// Run immediately on start.
	t.execute(ctx)

	ticker := time.NewTicker(t.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.logger.Info("task stopping (context cancelled)")
			return
		case <-ticker.C:
			t.execute(ctx)
		}
	}
}

// execute performs a single invocation and logs the outcome.
func (t *Task) execute(ctx context.Context) {
	start := time.Now()
	if err := t.RunFunc(ctx); err != nil {
		t.logger.WithError(err).WithField("duration", time.Since(start).Round(time.Millisecond)).
			Error("task execution failed")
	} else {
		t.logger.WithField("duration", time.Since(start).Round(time.Millisecond)).
			Debug("task execution completed")
	}
}
