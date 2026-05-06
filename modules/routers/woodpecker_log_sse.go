package routers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/emicklei/go-restful/v3"

	"github.com/thepenn/devsys/model"
	pipelinesvc "github.com/thepenn/devsys/service/pipeline"
)

const (
	woodpeckerLogIdlePing  = 30 * time.Second
	woodpeckerLogChanDepth = 10
	woodpeckerBatchDepth   = 30
)

type woodpeckerLogLineJSON struct {
	ID      int64  `json:"id,omitempty"`
	Line    int    `json:"line"`
	Type    int    `json:"type"`
	Time    int64  `json:"time"`
	Out     string `json:"out"`
	Content string `json:"content"`
}

func marshalWoodpeckerLogLine(e *model.LogEntry) ([]byte, error) {
	if e == nil {
		return nil, errors.New("nil log entry")
	}
	text := string(e.Data)
	w := woodpeckerLogLineJSON{
		ID:      e.ID,
		Line:    e.Line,
		Type:    int(e.Type),
		Time:    e.Time,
		Out:     text,
		Content: text,
	}
	return json.Marshal(w)
}

func writeWoodpeckerSSEError(rw http.ResponseWriter, flusher http.Flusher, msg string) {
	_, _ = fmt.Fprintf(rw, "event: error\ndata: %s\n\n", msg)
	flusher.Flush()
}

// writeWoodpeckerStepLogSSE streams one step's live logs in Woodpecker-compatible SSE
// (see https://github.com/woodpecker-ci/woodpecker server/api/stream.go LogStreamSSE).
func writeWoodpeckerStepLogSSE(resp *restful.Response, req *restful.Request, pl *pipelinesvc.Service, step *model.Step) {
	rw := resp.ResponseWriter
	hdr := rw.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache")
	hdr.Set("Connection", "keep-alive")
	hdr.Set("X-Accel-Buffering", "no")
	resp.WriteHeader(http.StatusOK)
	flusher, ok := rw.(http.Flusher)
	if !ok {
		return
	}
	_, _ = io.WriteString(rw, ": ping\n\n")
	flusher.Flush()

	if step == nil {
		writeWoodpeckerSSEError(rw, flusher, "process not found")
		return
	}
	if step.State != model.StatusPending && step.State != model.StatusRunning {
		writeWoodpeckerSSEError(rw, flusher, "step not running (anymore)")
		return
	}

	requestCtx := req.Request.Context()

	tailCtx, cancelTail := context.WithCancelCause(context.Background())
	defer cancelTail(nil)

	logChan := make(chan []byte, woodpeckerLogChanDepth)

	go func() {
		_ = pl.LogMuxOpen(tailCtx, step.ID)
		batches := make(chan []*model.LogEntry, woodpeckerBatchDepth)
		var innerDone sync.WaitGroup
		innerDone.Add(1)
		go func() {
			defer innerDone.Done()
			for entries := range batches {
				for _, entry := range entries {
					if entry == nil {
						continue
					}
					buf, err := marshalWoodpeckerLogLine(entry)
					if err != nil {
						continue
					}
					select {
					case <-tailCtx.Done():
						return
					case logChan <- buf:
					}
				}
			}
		}()

		tailErr := pl.LogMuxTail(tailCtx, step.ID, batches)
		close(batches)
		innerDone.Wait()
		cancelTail(tailErr)
	}()

	last, _ := strconv.Atoi(req.Request.Header.Get("Last-Event-ID"))
	id := 1

	for {
		select {
		case <-tailCtx.Done():
			if err := context.Cause(tailCtx); errors.Is(err, context.Canceled) {
				_, _ = io.WriteString(rw, "event: eof\ndata: eof\n\n")
				flusher.Flush()
			}
			return
		case <-requestCtx.Done():
			return
		case <-time.After(woodpeckerLogIdlePing):
			_, _ = io.WriteString(rw, ": ping\n\n")
			flusher.Flush()
		case buf, ok := <-logChan:
			if !ok {
				return
			}
			if id > last {
				_, _ = io.WriteString(rw, "id: "+strconv.Itoa(id)+"\n")
				_, _ = io.WriteString(rw, "data: ")
				_, _ = rw.Write(buf)
				_, _ = io.WriteString(rw, "\n\n")
				flusher.Flush()
			}
			id++
		}
	}
}
