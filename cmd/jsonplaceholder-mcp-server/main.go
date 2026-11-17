// Command jsonplaceholder-mcp-server is the composition root: it is the only
// place that knows every adapter and wires them into the application.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/adapters/inbound/mcpserver"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/adapters/outbound/jsonplaceholder"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/application"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/bootstrap"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/config"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/version"
)

// telemetryFlushTimeout bounds how long shutdown waits to flush buffered spans.
const telemetryFlushTimeout = 5 * time.Second

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
	ctx := context.Background()

	// Telemetry first: the SDK and the MCP adapter pick the providers up from
	// the globals when they are built.
	rt, err := bootstrap.Setup(ctx, cfg, version.Version)
	if err != nil {
		return err
	}
	defer flushTelemetry(rt)

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

	slog.Info("starting", "version", version.Version, "transport", cfg.Transport, "api_host", cfg.APIHost,
		"telemetry", rt.Mode.String(), "metrics_port", cfg.MetricsPort)
	if cfg.Transport == config.Stdio {
		return server.ServeStdio(mcpSrv)
	}
	handler := bootstrap.WrapHTTP(mcpserver.NewHTTPHandler(mcpSrv, config.EndpointPath), rt)
	return bootstrap.ServeHTTP(ctx, cfg, handler)
}

// flushTelemetry flushes buffered spans and stops the metrics listener.
func flushTelemetry(rt *bootstrap.Runtime) {
	ctx, cancel := context.WithTimeout(context.Background(), telemetryFlushTimeout)
	defer cancel()
	if err := rt.Shutdown(ctx); err != nil {
		slog.Error("telemetry shutdown", "err", err)
	}
}
