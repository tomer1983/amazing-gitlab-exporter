package scheduler

import (
	"context"

	"github.com/sirupsen/logrus"
)

// TaskQueue is a simple in-memory FIFO queue for on-demand task execution
// (e.g., webhook-triggered refreshes). It is a placeholder that can be
// extended in the future to use Redis for distributed queueing.
type TaskQueue struct {
	ch     chan func()
	logger *logrus.Entry
}

// NewTaskQueue creates a new in-memory task queue with the given buffer size.
func NewTaskQueue(bufferSize int, logger *logrus.Entry) *TaskQueue {
	if bufferSize < 1 {
		bufferSize = 64
	}
	return &TaskQueue{
		ch:     make(chan func(), bufferSize),
		logger: logger.WithField("component", "task_queue"),
	}
}

// Enqueue adds fn to the queue for asynchronous execution. If the queue is
// full the function is dropped and a warning is logged.
func (q *TaskQueue) Enqueue(fn func()) {
	select {
	case q.ch <- fn:
		q.logger.Debug("task enqueued")
	default:
		q.logger.Warn("task queue full, dropping task")
	}
}

// Start processes queued tasks sequentially until ctx is cancelled.
func (q *TaskQueue) Start(ctx context.Context) {
	q.logger.Info("task queue started")
	for {
		select {
		case <-ctx.Done():
			q.logger.Info("task queue stopping (context cancelled)")
			return
		case fn := <-q.ch:
			func() {
				defer func() {
					if r := recover(); r != nil {
						q.logger.WithField("panic", r).Error("queued task panicked")
					}
				}()
				fn()
			}()
		}
	}
}
