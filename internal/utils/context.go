package utils

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func WithContext(ctx context.Context) context.Context {
	return WithContextFunc(ctx, func() {
		println("interrupt received, terminating process")
	})
}

func WithContextFunc(ctx context.Context, f func()) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(c)

		select {
		case <-ctx.Done():
		case <-c:
			f()
			cancel()
		}
	}()

	return ctx
}
