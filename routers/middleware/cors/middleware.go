package cors

import (
	"net/http"
)

type Middleware struct {
	allowedHeaders string
	allowedMethods string
}

func New() *Middleware {
	return &Middleware{
		allowedHeaders: "Content-Type, Authorization, X-Requested-With, Accept, Origin",
		allowedMethods: "GET, POST, PUT, PATCH, DELETE, OPTIONS",
	}
}

func (m *Middleware) wrapHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Del("Access-Control-Allow-Credentials")
	} else {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Headers", m.allowedHeaders)
	w.Header().Set("Access-Control-Allow-Methods", m.allowedMethods)
}

func (m *Middleware) WrapHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.wrapHeaders(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
