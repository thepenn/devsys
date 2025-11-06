package model

import (
	"errors"
	"fmt"
)

type WebhookEvent string

const (
	EventPush         WebhookEvent = "push"
	EventPull         WebhookEvent = "pull_request"
	EventPullClosed   WebhookEvent = "pull_request_closed"
	EventPullMetadata WebhookEvent = "pull_request_metadata"
	EventTag          WebhookEvent = "tag"
	EventRelease      WebhookEvent = "release"
	EventDeploy       WebhookEvent = "deployment"
	EventCron         WebhookEvent = "cron"
	EventManual       WebhookEvent = "manual"
)

type WebhookEventList []WebhookEvent

func (wel WebhookEventList) Len() int           { return len(wel) }
func (wel WebhookEventList) Swap(i, j int)      { wel[i], wel[j] = wel[j], wel[i] }
func (wel WebhookEventList) Less(i, j int) bool { return wel[i] < wel[j] }

var ErrInvalidWebhookEvent = errors.New("invalid webhook event")

func (s WebhookEvent) Validate() error {
	switch s {
	case EventPush, EventPull, EventPullClosed, EventPullMetadata, EventTag, EventRelease, EventDeploy, EventCron, EventManual:
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrInvalidWebhookEvent, s)
	}
}

type StatusValue string

const (
	StatusSkipped  StatusValue = "skipped"
	StatusPending  StatusValue = "pending"
	StatusRunning  StatusValue = "running"
	StatusSuccess  StatusValue = "success"
	StatusFailure  StatusValue = "failure"
	StatusKilled   StatusValue = "killed"
	StatusError    StatusValue = "error"
	StatusBlocked  StatusValue = "blocked"
	StatusDeclined StatusValue = "declined"
	StatusCreated  StatusValue = "created"
)

var ErrInvalidStatusValue = errors.New("invalid status value")

func (s StatusValue) Validate() error {
	switch s {
	case StatusSkipped, StatusPending, StatusRunning, StatusSuccess, StatusFailure, StatusKilled, StatusError, StatusBlocked, StatusDeclined, StatusCreated:
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrInvalidStatusValue, s)
	}
}

type RepoVisibility string

const (
	VisibilityPublic   RepoVisibility = "public"
	VisibilityPrivate  RepoVisibility = "private"
	VisibilityInternal RepoVisibility = "internal"
)
