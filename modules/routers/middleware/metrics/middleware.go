package metrics

import (
	"context"
	"time"

	"github.com/emicklei/go-restful/v3"
)

type ctxKey string

const startTimeKey ctxKey = "startTime"

// Middleware provides request scoped metrics helpers.
type Middleware struct{}

func New() *Middleware {
	return &Middleware{}
}

func (m *Middleware) Middleware() []restful.FilterFunction {
	return []restful.FilterFunction{
		m.injectStartTime,
	}
}

func (m *Middleware) injectStartTime(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
	ctx := context.WithValue(req.Request.Context(), startTimeKey, time.Now())
	req.Request = req.Request.WithContext(ctx)
	chain.ProcessFilter(req, resp)
}

// StartTimeFromContext extracts the request start time from context.
func StartTimeFromContext(ctx context.Context) (time.Time, bool) {
	value := ctx.Value(startTimeKey)
	if ts, ok := value.(time.Time); ok {
		return ts, true
	}
	return time.Time{}, false
}
