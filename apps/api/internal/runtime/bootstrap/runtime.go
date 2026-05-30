package bootstrap

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"api/internal/platform/config"
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
		log.Printf("[Main] Asynq worker starting: queue=%s concurrency=%d", a.container.workerQueueName, a.container.workerConcurrent)
		if err := a.container.asynqServer.Run(a.container.asynqMux); err != nil {
			log.Fatalf("[Main] Failed to start asynq worker: %v", err)
		}
	}()

	go func() {
		log.Printf("Server starting on %s", a.httpServer.Addr)
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()
}

func (a *runtimeApp) waitForShutdownSignal() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
}

func (a *runtimeApp) shutdown() error {
	log.Println("[Main] Shutting down server...")
	a.container.asynqServer.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.httpServer.Shutdown(ctx); err != nil {
		return err
	}

	log.Println("[Main] Server exited")
	return nil
}

func (a *runtimeApp) close() error {
	if a == nil || a.container == nil {
		return nil
	}
	return a.container.close()
}
