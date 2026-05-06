// Package logmux provides an in-memory multiplexer for pipeline step logs,
// modeled after Woodpecker's server/logging logger (Open / Write / Tail / Close).
package logmux

import (
	"context"
	"errors"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/thepenn/devsys/model"
)

// ErrNotFound is returned when Tail is called before Open created a stream.
var ErrNotFound = errors.New("logmux: stream not found")

const maxQueuedBatchesPerClient = 30

type subscriber struct {
	recv chan<- []*model.LogEntry
}

type stream struct {
	mu     sync.Mutex
	list   []*model.LogEntry
	subs   map[*subscriber]struct{}
	done   chan struct{}
	closed bool
}

// Mux multiplexes log batches per step ID for live SSE subscribers.
type Mux struct {
	mu      sync.Mutex
	streams map[int64]*stream
}

// New returns a new multiplexer.
func New() *Mux {
	return &Mux{
		streams: make(map[int64]*stream),
	}
}

// openStreamLocked returns the stream for stepID; m.mu must be held.
func (m *Mux) openStreamLocked(stepID int64) *stream {
	if s, ok := m.streams[stepID]; ok {
		return s
	}
	s := &stream{
		subs: make(map[*subscriber]struct{}),
		done: make(chan struct{}),
	}
	m.streams[stepID] = s
	return s
}

// Open ensures a stream exists for the step (Woodpecker Logs.Open).
func (m *Mux) Open(_ context.Context, stepID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.openStreamLocked(stepID)
	return nil
}

// Write appends entries to the buffer and pushes batches to subscribers (Woodpecker Logs.Write).
func (m *Mux) Write(_ context.Context, stepID int64, entries []*model.LogEntry) error {
	if len(entries) == 0 {
		return nil
	}
	batch := make([]*model.LogEntry, len(entries))
	for i := range entries {
		e := *entries[i]
		batch[i] = &e
	}

	m.mu.Lock()
	s := m.openStreamLocked(stepID)
	m.mu.Unlock()

	s.mu.Lock()
	s.list = append(s.list, batch...)
	subs := make([]chan<- []*model.LogEntry, 0, len(s.subs))
	for sub := range s.subs {
		subs = append(subs, sub.recv)
	}
	s.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- batch:
		default:
			log.Info().Int64("step_id", stepID).Msg("logmux: subscriber channel full, dropping batch")
		}
	}
	return nil
}

// Tail subscribes to log batches until ctx or the stream is closed (Woodpecker Logs.Tail).
// The batches channel must have capacity maxQueuedBatchesPerClient; replay is sent first.
func (m *Mux) Tail(ctx context.Context, stepID int64, batches chan<- []*model.LogEntry) error {
	m.mu.Lock()
	s, ok := m.streams[stepID]
	m.mu.Unlock()
	if !ok {
		return ErrNotFound
	}

	sub := &subscriber{recv: batches}

	s.mu.Lock()
	s.subs[sub] = struct{}{}
	var replay []*model.LogEntry
	if len(s.list) > 0 {
		replay = append([]*model.LogEntry(nil), s.list...)
	}
	s.mu.Unlock()

	if len(replay) > 0 {
		select {
		case batches <- replay:
		case <-ctx.Done():
			s.mu.Lock()
			delete(s.subs, sub)
			s.mu.Unlock()
			return ctx.Err()
		case <-s.done:
			s.mu.Lock()
			delete(s.subs, sub)
			s.mu.Unlock()
			return nil
		}
	}

	select {
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.subs, sub)
		s.mu.Unlock()
		return ctx.Err()
	case <-s.done:
		s.mu.Lock()
		delete(s.subs, sub)
		s.mu.Unlock()
		return nil
	}
}

// Close ends the stream for a step (Woodpecker Logs.Close).
func (m *Mux) Close(_ context.Context, stepID int64) error {
	m.mu.Lock()
	s, ok := m.streams[stepID]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	delete(m.streams, stepID)
	m.mu.Unlock()

	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.done)
	}
	s.mu.Unlock()
	return nil
}
