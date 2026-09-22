package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/Alisher24/CarSharing/backend/internal/platform/lifecycle"
)

// Serve closes every listener on cancellation or any listener failure and joins their serving tasks.
func Serve(ctx context.Context, servers ...*http.Server) error {
	finished := make(chan error, len(servers))
	for _, server := range servers {
		go func() { finished <- listen(server) }()
	}
	var failure error
	remaining := len(servers)
	select {
	case failure = <-finished:
		remaining--
	case <-ctx.Done():
	}
	failure = errors.Join(failure, shutdown(servers))
	for range remaining {
		failure = errors.Join(failure, <-finished)
	}
	return failure
}

func listen(server *http.Server) error {
	slog.Info("HTTP listener started", "address", server.Addr)
	err := server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("the listener on %s failed: %w", server.Addr, err)
	}
	return nil
}

func shutdown(servers []*http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), lifecycle.ShutdownTimeout)
	defer cancel()
	var failure error
	for _, server := range servers {
		if err := server.Shutdown(ctx); err != nil {
			_ = server.Close()
			failure = errors.Join(failure, fmt.Errorf("HTTP shutdown on %s: %w", server.Addr, err))
		}
	}
	slog.Info("HTTP listeners stopped")
	return failure
}
