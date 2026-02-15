// Package scheduler provides a simple task scheduler that runs collectors
// at configured intervals, with optional on-demand execution via a task queue.
package scheduler

import (
	"context"
	"sync"

	"github.com/sirupsen/logrus"
)

// Scheduler manages a set of periodic tasks, running each in its own goroutine.
type Scheduler struct {
	tasks  []*Task
	logger *logrus.Entry
	wg     sync.WaitGroup
	cancel context.CancelFunc
}

// NewScheduler creates a new scheduler.
func NewScheduler(logger *logrus.Entry) *Scheduler {
	return &Scheduler{
		logger: logger.WithField("component", "scheduler"),
	}
}

// AddTask registers a task to be started when Start is called.
// It must be called before Start.
func (s *Scheduler) AddTask(task *Task) {
	s.tasks = append(s.tasks, task)
}

// Start launches a goroutine for every registered task. Each goroutine runs
// the task's loop (see Task.Run) until ctx is cancelled or Stop is called.
func (s *Scheduler) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)

	s.logger.WithField("task_count", len(s.tasks)).Info("starting scheduler")

	for _, t := range s.tasks {
		s.wg.Add(1)
		go func(task *Task) {
			defer s.wg.Done()
			task.Run(ctx)
		}(t)
	}
}

// Stop cancels all running tasks and blocks until every goroutine has returned.
func (s *Scheduler) Stop() {
	s.logger.Info("stopping scheduler")
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	s.logger.Info("scheduler stopped")
}
