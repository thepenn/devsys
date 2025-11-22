package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"

	"github.com/thepenn/devsys/model"
)

var (
	// ErrQueueClosed is returned when the queue is shutting down or already stopped.
	ErrQueueClosed = errors.New("pipeline queue closed")
	// ErrQueueNotStarted indicates Start has not been called yet.
	ErrQueueNotStarted = errors.New("pipeline queue not started")
	// ErrInvalidWorkerCount is returned when workers <= 0.
	ErrInvalidWorkerCount = errors.New("worker count must be greater than zero")
)

// Executor defines the signature for processing tasks pulled from the queue.
type Executor func(context.Context, *model.Task) error

// Stats provides insight into the current queue state.
type Stats struct {
	Running       bool
	Workers       int
	Pending       int
	InFlight      int
	EnqueuedTotal uint64
	Processed     uint64
}

// PipelineQueue handles asynchronous task dispatch for pipelines.
type PipelineQueue struct {
	tasks   chan *model.Task
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started atomic.Bool
	closed  atomic.Bool

	enqueueCount   atomic.Uint64
	processedCount atomic.Uint64
	workerCount    atomic.Int32
	inflight       atomic.Int32
}

// New creates a queue with the provided capacity.
func New(capacity int) *PipelineQueue {
	if capacity <= 0 {
		capacity = 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &PipelineQueue{
		tasks:  make(chan *model.Task, capacity),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start launches worker goroutines that pull tasks from the queue.
func (q *PipelineQueue) Start(parent context.Context, workers int, executor Executor) error {
	if workers <= 0 {
		return ErrInvalidWorkerCount
	}

	if !q.started.CompareAndSwap(false, true) {
		return nil
	}

	q.workerCount.Store(int32(workers))

	go func() {
		select {
		case <-parent.Done():
			q.Shutdown()
		case <-q.ctx.Done():
		}
	}()

	for i := 0; i < workers; i++ {
		q.wg.Add(1)
		go q.worker(i+1, executor)
	}

	log.Info().Int("workers", workers).Msg("pipeline queue started")

	return nil
}

// Enqueue adds a task to the queue for asynchronous processing.
func (q *PipelineQueue) Enqueue(ctx context.Context, task *model.Task) error {
	if task == nil {
		return fmt.Errorf("queue: task is nil")
	}
	if !q.started.Load() {
		return ErrQueueNotStarted
	}
	if q.closed.Load() {
		return ErrQueueClosed
	}

	select {
	case <-q.ctx.Done():
		return ErrQueueClosed
	case <-ctx.Done():
		return ctx.Err()
	case q.tasks <- task:
		q.enqueueCount.Add(1)
		return nil
	}
}

// Stats returns queue statistics.
func (q *PipelineQueue) Stats() Stats {
	return Stats{
		Running:       q.started.Load() && !q.closed.Load(),
		Workers:       int(q.workerCount.Load()),
		Pending:       len(q.tasks),
		InFlight:      int(q.inflight.Load()),
		EnqueuedTotal: q.enqueueCount.Load(),
		Processed:     q.processedCount.Load(),
	}
}

// Shutdown stops workers gracefully. It is safe to call multiple times.
func (q *PipelineQueue) Shutdown() {
	if q.closed.CompareAndSwap(false, true) {
		q.cancel()
		close(q.tasks)
		q.wg.Wait()
		log.Info().Msg("pipeline queue stopped")
	}
}

func (q *PipelineQueue) worker(id int, executor Executor) {
	defer q.wg.Done()
	workerLogger := log.With().Int("worker", id).Logger()

	for {
		select {
		case <-q.ctx.Done():
			workerLogger.Debug().Msg("worker context canceled")
			return
		case task, ok := <-q.tasks:
			if !ok {
				workerLogger.Debug().Msg("task channel closed")
				return
			}
			if task == nil {
				continue
			}

			q.inflight.Add(1)
			if err := executor(q.ctx, task); err != nil {
				workerLogger.Error().Err(err).Str("task", task.ID).Msg("failed to execute task")
			}
			q.processedCount.Add(1)
			q.inflight.Add(-1)
		}
	}
}
