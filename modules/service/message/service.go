// Package message owns the messages persistence layer.
//
// Other services emit notifications via Service#Create. The recipient reads
// them through the messages REST endpoints. Messages are per-user (UserID
// is required); broadcasting to a group is intentionally out of scope —
// the caller would emit one Message row per recipient.
package message

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/thepenn/devsys/internal/store"
	"github.com/thepenn/devsys/model"
)

// Service exposes CRUD on the messages table.
type Service struct {
	db *store.DB
}

// New constructs a message service.
func New(db *store.DB) *Service {
	return &Service{db: db}
}

// CreateInput is the payload used by Create.
type CreateInput struct {
	UserID  int64
	Type    string // model.MessageType*
	Source  string // model.MessageSource*
	Title   string
	Content string
}

// Create inserts a single notification. Empty Type/Source default to info/system
// so callers don't have to import the model package for every emit.
func (s *Service) Create(ctx context.Context, in CreateInput) (*model.Message, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("message service not initialised")
	}
	if in.UserID == 0 {
		return nil, errors.New("user id is required")
	}
	if strings.TrimSpace(in.Title) == "" {
		return nil, errors.New("title is required")
	}
	msg := &model.Message{
		UserID:  in.UserID,
		Type:    nonEmpty(in.Type, model.MessageTypeInfo),
		Source:  nonEmpty(in.Source, model.MessageSourceSystem),
		Title:   in.Title,
		Content: in.Content,
		Created: time.Now().Unix(),
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Create(msg).Error
	})
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// ListOptions filters supported by List.
type ListOptions struct {
	Page       int
	PerPage    int
	UnreadOnly bool
}

// ListResult is the paginated response shape.
type ListResult struct {
	Items   []model.Message `json:"items"`
	Total   int64           `json:"total"`
	Unread  int64           `json:"unread"`
	Page    int             `json:"page"`
	PerPage int             `json:"per_page"`
}

// List returns a page of messages addressed to userID, ordered by id DESC.
// Always returns total + unread counts so the UI can render a badge.
func (s *Service) List(ctx context.Context, userID int64, opts ListOptions) (*ListResult, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("message service not initialised")
	}
	if userID == 0 {
		return nil, errors.New("user id is required")
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
		items  []model.Message
		total  int64
		unread int64
	)
	err := s.db.View(func(tx *gorm.DB) error {
		ctxTx := tx.WithContext(ctx)
		base := ctxTx.Model(&model.Message{}).Where("user_id = ?", userID)
		filtered := base
		if opts.UnreadOnly {
			filtered = filtered.Where("read_at = 0")
		}
		if err := filtered.Count(&total).Error; err != nil {
			return err
		}
		if err := base.Where("read_at = 0").Count(&unread).Error; err != nil {
			return err
		}
		return filtered.Order("id DESC").
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
		Unread:  unread,
		Page:    opts.Page,
		PerPage: opts.PerPage,
	}, nil
}

// UnreadCount returns the number of unread messages addressed to userID.
// Cheap query for badge display in the global header.
func (s *Service) UnreadCount(ctx context.Context, userID int64) (int64, error) {
	if s == nil || s.db == nil || userID == 0 {
		return 0, nil
	}
	var n int64
	err := s.db.View(func(tx *gorm.DB) error {
		return tx.WithContext(ctx).Model(&model.Message{}).
			Where("user_id = ? AND read_at = 0", userID).
			Count(&n).Error
	})
	return n, err
}

// MarkRead flips read_at = now for the supplied message ids belonging to userID.
// Messages that are already read remain unchanged. Returns the number of rows
// that were actually updated.
func (s *Service) MarkRead(ctx context.Context, userID int64, ids []int64) (int64, error) {
	if userID == 0 || len(ids) == 0 {
		return 0, nil
	}
	now := time.Now().Unix()
	var affected int64
	err := s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.WithContext(ctx).Model(&model.Message{}).
			Where("user_id = ? AND id IN ? AND read_at = 0", userID, ids).
			Update("read_at", now)
		if res.Error != nil {
			return res.Error
		}
		affected = res.RowsAffected
		return nil
	})
	return affected, err
}

// MarkAllRead flips every unread message of userID to read; useful for the
// "mark all" UI button.
func (s *Service) MarkAllRead(ctx context.Context, userID int64) (int64, error) {
	if userID == 0 {
		return 0, nil
	}
	now := time.Now().Unix()
	var affected int64
	err := s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.WithContext(ctx).Model(&model.Message{}).
			Where("user_id = ? AND read_at = 0", userID).
			Update("read_at", now)
		if res.Error != nil {
			return res.Error
		}
		affected = res.RowsAffected
		return nil
	})
	return affected, err
}

func nonEmpty(value, fallback string) string {
	if v := strings.TrimSpace(value); v != "" {
		return v
	}
	return fallback
}
