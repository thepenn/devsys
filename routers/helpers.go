package routers

import (
	"github.com/emicklei/go-restful/v3"
)

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(resp *restful.Response, status int, err error) {
	_ = resp.WriteHeaderAndEntity(status, errorResponse{Error: err.Error()})
}
