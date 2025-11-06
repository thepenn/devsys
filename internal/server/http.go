package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

type HttpServer struct {
	Addr    string
	Handler http.Handler
}

func NewHttpServer(addr string, handler http.Handler) *HttpServer {
	return &HttpServer{
		Addr:    addr,
		Handler: handler,
	}
}

func (s *HttpServer) ListenAndServe(ctx context.Context) error {
	err := s.listenAndServe(ctx)
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}

	return err
}

func (s *HttpServer) listenAndServe(ctx context.Context) error {
	if err := s.checkPortAvailable(); err != nil {
		log.Error().Err(err).Str("addr", s.Addr).Msg("port is already in use")
		panic(fmt.Sprintf("Port %s is already in use: %v", s.Addr, err))
	}

	var g errgroup.Group

	httpServer := &http.Server{
		Addr:         s.Addr,
		Handler:      s.Handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	g.Go(func() error {
		<-ctx.Done()
		log.Info().Msg("shutting down http server")

		ctxShutdown, cancelFunc := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelFunc()

		return httpServer.Shutdown(ctxShutdown)
	})
	g.Go(func() error {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("http server failed")
			return err
		}

		return nil
	})

	return g.Wait()
}

func (s *HttpServer) checkPortAvailable() error {
	listener, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return fmt.Errorf("port %s is already in use", s.Addr)
	}
	defer listener.Close()
	return nil
}
