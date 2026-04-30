// Package audit owns the audit_logs persistence layer.
//
// 设计目标:
//   1. 写路径不能阻塞 HTTP 请求 (中间件在 chain.ProcessFilter 之后调用 Record).
//   2. 高 QPS 下避免每条一次 INSERT, 走批量.
//   3. 服务关闭时把缓冲里残留的 entry 落库, 不丢日志.
//
// 实现方式: Record() 把 entry 投入一个 buffered channel; 后台 worker 在 batchSize
// 满或 flushInterval 到期时 INSERT 一批; 服务 Stop() 关闭 channel 并等待 worker
// 退出.
package audit

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/thepenn/devsys/internal/store"
	"github.com/thepenn/devsys/model"
)

const (
	defaultBufferSize    = 1024
	defaultBatchSize     = 100
	defaultFlushInterval = 5 * time.Second
)

// Service exposes the write entry-point (Record) used by the audit middleware
// and the read entry-point (List) used by the management UI.
type Service struct {
	db *store.DB

	ch       chan model.AuditLog
	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// New starts the background worker. Stop must be called on shutdown to flush
// pending entries.
func New(db *store.DB) *Service {
	s := &Service{
		db:     db,
		ch:     make(chan model.AuditLog, defaultBufferSize),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	go s.worker()
	return s
}

// Record enqueues an audit entry. Falls back to a synchronous best-effort
// insert if the channel is full so we never lose a record under burst load
// (at the cost of a brief latency spike for that one request).
func (s *Service) Record(entry model.AuditLog) {
	if s == nil || s.db == nil {
		return
	}
	if entry.Created == 0 {
		entry.Created = time.Now().Unix()
	}
	select {
	case s.ch <- entry:
	default:
		// Buffer full: drop into DB directly so we don't drop the entry.
		// Errors are logged, not propagated, since the original request
		// has already returned to the client.
		s.flush([]model.AuditLog{entry})
	}
}

// Stop signals the worker to drain remaining entries and exit. Safe to call
// multiple times.
func (s *Service) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	<-s.doneCh
}

func (s *Service) worker() {
	defer close(s.doneCh)
	batch := make([]model.AuditLog, 0, defaultBatchSize)
	ticker := time.NewTicker(defaultFlushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		s.flush(batch)
		batch = batch[:0]
	}

	for {
		select {
		case entry := <-s.ch:
			batch = append(batch, entry)
			if len(batch) >= defaultBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-s.stopCh:
			// Drain channel before exiting to avoid losing entries that were
			// queued during shutdown.
			for {
				select {
				case entry := <-s.ch:
					batch = append(batch, entry)
				default:
					flush()
					return
				}
			}
		}
	}
}

func (s *Service) flush(batch []model.AuditLog) {
	if len(batch) == 0 {
		return
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// CreateInBatches splits into chunks; a 100-row batch fits well within
		// MySQL's default max_allowed_packet so we use a single batch here.
		return tx.CreateInBatches(batch, len(batch)).Error
	})
	if err != nil {
		log.Error().Err(err).Int("count", len(batch)).Msg("audit: batch insert failed")
	}
}

// ListOptions filters supported by List.
type ListOptions struct {
	Page    int
	PerPage int
	UserID  int64  // 0 = no filter
	Login   string // partial, case-insensitive
	Method  string // exact, upper-cased
	Path    string // partial
	Start   int64  // unix sec, inclusive
	End     int64  // unix sec, exclusive
}

// ListResult is the paginated response shape.
type ListResult struct {
	Items   []model.AuditLog `json:"items"`
	Total   int64            `json:"total"`
	Page    int              `json:"page"`
	PerPage int              `json:"per_page"`
}

// List returns a page of audit log rows ordered by id DESC (newest first).
func (s *Service) List(ctx context.Context, opts ListOptions) (*ListResult, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("audit service not initialised")
	}
	if opts.Page <= 0 {
		opts.Page = 1
	}
	if opts.PerPage <= 0 {
		opts.PerPage = 20
	}
	if opts.PerPage > 200 {
		opts.PerPage = 200
	}

	var (
		items []model.AuditLog
		total int64
	)
	err := s.db.View(func(tx *gorm.DB) error {
		q := tx.WithContext(ctx).Model(&model.AuditLog{})
		if opts.UserID > 0 {
			q = q.Where("user_id = ?", opts.UserID)
		}
		if v := strings.TrimSpace(opts.Login); v != "" {
			q = q.Where("login LIKE ?", "%"+v+"%")
		}
		if v := strings.TrimSpace(strings.ToUpper(opts.Method)); v != "" {
			q = q.Where("method = ?", v)
		}
		if v := strings.TrimSpace(opts.Path); v != "" {
			q = q.Where("path LIKE ?", "%"+v+"%")
		}
		if opts.Start > 0 {
			q = q.Where("created >= ?", opts.Start)
		}
		if opts.End > 0 {
			q = q.Where("created < ?", opts.End)
		}
		if err := q.Count(&total).Error; err != nil {
			return err
		}
		return q.Order("id DESC").
			Offset((opts.Page - 1) * opts.PerPage).
			Limit(opts.PerPage).
			Find(&items).Error
	})
	if err != nil {
		return nil, err
	}
	return &ListResult{
		Items:   items,
		Total:   total,
		Page:    opts.Page,
		PerPage: opts.PerPage,
	}, nil
}
