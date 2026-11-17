// Command jsonplaceholder-mcp-server is the composition root: it is the only
// place that knows every adapter and wires them into the application.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/adapters/inbound/mcpserver"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/adapters/outbound/jsonplaceholder"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/application"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/config"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/version"
)

const shutdownGrace = 5 * time.Second

func main() {
	if err := run(); err != nil {
		// stderr, never stdout: stdout is the JSON-RPC stream under stdio.
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel})))

	source, err := jsonplaceholder.New(cfg.APIHost, &http.Client{Timeout: cfg.HTTPTimeout})
	if err != nil {
		return err
	}
	svc, err := application.NewService(source)
	if err != nil {
		return err
	}
	mcpSrv, err := mcpserver.New(svc, version.Version)
	if err != nil {
		return err
	}

	slog.Info("starting", "version", version.Version, "transport", cfg.Transport, "api_host", cfg.APIHost)
	if cfg.Transport == config.Stdio {
		return server.ServeStdio(mcpSrv)
	}
	return serveHTTP(cfg, mcpserver.NewHTTPHandler(mcpSrv, config.EndpointPath))
}

func serveHTTP(cfg config.Config, handler http.Handler) error {
	srv := &http.Server{
		Addr:              net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	slog.Info("listening", "addr", srv.Addr, "endpoint", config.EndpointPath)

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	shutCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}
