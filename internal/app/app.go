// Package app composes process lifecycle and foundation services.
package app

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/podopodo/db_accelerator/internal/buildinfo"
	"github.com/podopodo/db_accelerator/internal/config"
	"github.com/podopodo/db_accelerator/internal/control"
	"github.com/podopodo/db_accelerator/internal/lifecycle"
)

type App struct {
	cfg       config.Config
	logger    *slog.Logger
	state     *lifecycle.Manager
	ready     chan struct{}
	readyOnce sync.Once
	mu        sync.RWMutex
	adminAddr string
}

func New(cfg config.Config, logger *slog.Logger) *App {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stderr, nil))
	}
	return &App{
		cfg:    cfg,
		logger: logger,
		state:  lifecycle.New(time.Now()),
		ready:  make(chan struct{}),
	}
}

func (a *App) Ready() <-chan struct{} { return a.ready }

func (a *App) State() *lifecycle.Manager { return a.state }

func (a *App) AdminAddress() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.adminAddr
}

// Run serves the foundation control endpoints until the context is cancelled.
func (a *App) Run(ctx context.Context) error {
	if err := os.MkdirAll(a.cfg.Server.DataDir, 0o700); err != nil {
		_ = a.state.Transition(lifecycle.Failed, "create data directory", time.Now())
		return err
	}

	listener, err := net.Listen("tcp", a.cfg.Server.AdminListen)
	if err != nil {
		_ = a.state.Transition(lifecycle.Failed, "open admin listener", time.Now())
		return err
	}
	a.mu.Lock()
	a.adminAddr = listener.Addr().String()
	a.mu.Unlock()

	server := &http.Server{
		Handler:           control.HealthHandler(a.state, buildinfo.Current()),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.Serve(listener)
	}()

	if err := a.state.Transition(lifecycle.Ready, "foundation services ready", time.Now()); err != nil {
		_ = listener.Close()
		return err
	}
	a.readyOnce.Do(func() { close(a.ready) })
	a.logger.Info("accelerator ready", "admin_listen", listener.Addr().String(), "version", buildinfo.Version)

	select {
	case <-ctx.Done():
		_ = a.state.Transition(lifecycle.Draining, "shutdown requested", time.Now())
	case serveErr := <-serveResult:
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			_ = a.state.Transition(lifecycle.Failed, "admin server failed", time.Now())
			return serveErr
		}
		_ = a.state.Transition(lifecycle.Draining, "admin server stopped", time.Now())
	}

	shutdownDuration, _ := a.cfg.ShutdownDuration()
	shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownDuration)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		_ = a.state.Transition(lifecycle.Failed, "graceful shutdown failed", time.Now())
		_ = server.Close()
		return err
	}
	if err := a.state.Transition(lifecycle.Stopped, "shutdown complete", time.Now()); err != nil {
		return err
	}
	a.logger.Info("accelerator stopped")
	return nil
}
