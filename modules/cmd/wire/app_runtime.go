package wire

import (
	"context"
	"errors"
)

// Start initialises background services.
func (a *App) Start(ctx context.Context) error {
	if a.Services != nil && a.Services.Pipeline != nil {
		return a.Services.Pipeline.Start(ctx)
	}
	return nil
}

// Close releases resources.
func (a *App) Close() error {
	var errs []error
	if a.Services != nil && a.Services.Pipeline != nil {
		a.Services.Pipeline.Shutdown()
	}
	if a.Cache != nil {
		a.Cache.Close()
	}
	if a.DB != nil {
		if err := a.DB.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
