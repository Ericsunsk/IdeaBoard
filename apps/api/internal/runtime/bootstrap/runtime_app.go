package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ideaboard/api/internal/platform/config"
)

type runtimeApp struct {
	container     *appContainer
	httpServer    *http.Server
	metricsServer *http.Server
}

func newRuntimeApp(cfg *config.Config) (*runtimeApp, error) {
	container, err := newAppContainer(cfg)
	if err != nil {
		return nil, err
	}
	return &runtimeApp{
		container:     container,
		httpServer:    newHTTPServer(cfg, container.router),
		metricsServer: newMetricsServer(cfg),
	}, nil
}

func (a *runtimeApp) start() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("River worker goroutine panicked", "panic", r)
				fatal("River worker goroutine panicked", "panic", r)
			}
		}()
		slog.Info("River worker starting", "queue", a.container.workerRuntime.queueName, "concurrency", a.container.workerRuntime.concurrency)
		if err := a.container.workerRuntime.riverClient.Start(context.Background()); err != nil {
			fatal("failed to start River worker", "err", err)
		}
	}()

	startHTTPServer("HTTP", a.httpServer)
	startHTTPServer("metrics", a.metricsServer)
}

func startHTTPServer(name string, server *http.Server) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error(name+" server goroutine panicked", "panic", r)
				fatal(name+" server goroutine panicked", "panic", r)
			}
		}()
		slog.Info(name+" server starting", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fatal("failed to start "+name+" server", "err", err)
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
	if err := a.container.workerRuntime.riverClient.Stop(stopCtx); err != nil {
		slog.Warn("River worker graceful stop failed", "err", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var shutdownErr error
	if err := a.httpServer.Shutdown(ctx); err != nil {
		shutdownErr = errors.Join(shutdownErr, err)
	}
	if err := a.metricsServer.Shutdown(ctx); err != nil {
		shutdownErr = errors.Join(shutdownErr, err)
	}

	slog.Info("server exited")
	return shutdownErr
}

func (a *runtimeApp) close() error {
	if a == nil || a.container == nil {
		return nil
	}
	return a.container.close()
}
