package bootstrap

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ideaboard/api/internal/platform/config"
)

type runtimeApp struct {
	container  *appContainer
	httpServer *http.Server
}

func newRuntimeApp(cfg *config.Config) (*runtimeApp, error) {
	container, err := newAppContainer(cfg)
	if err != nil {
		return nil, err
	}
	return &runtimeApp{
		container:  container,
		httpServer: newHTTPServer(cfg, container.router),
	}, nil
}

func (a *runtimeApp) start() {
	go func() {
		slog.Info("River worker starting", "queue", a.container.workerQueueName, "concurrency", a.container.workerConcurrent)
		if err := a.container.riverClient.Start(context.Background()); err != nil {
			fatal("failed to start River worker", "err", err)
		}
	}()

	go func() {
		slog.Info("HTTP server starting", "addr", a.httpServer.Addr)
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fatal("failed to start HTTP server", "err", err)
		}
	}()
}

func (a *runtimeApp) waitForShutdownSignal() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
}

func (a *runtimeApp) shutdown() error {
	slog.Info("server shutting down")
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()
	if err := a.container.riverClient.Stop(stopCtx); err != nil {
		slog.Warn("River worker graceful stop failed", "err", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.httpServer.Shutdown(ctx); err != nil {
		return err
	}

	slog.Info("server exited")
	return nil
}

func (a *runtimeApp) close() error {
	if a == nil || a.container == nil {
		return nil
	}
	return a.container.close()
}
