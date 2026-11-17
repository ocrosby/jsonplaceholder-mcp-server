// Package bootstrap is the application's observability and HTTP wiring: it
// decides which telemetry signals are on, starts go-platform's OpenTelemetry
// bootstrap and metrics listener, wraps the MCP handler in tracing and
// request middleware, and runs the HTTP server with graceful shutdown. It is
// the only package that knows go-platform's lifecycle packages, so main stays
// a plain composition root.
//
// STDIO NOTE: under the stdio transport stdout is the JSON-RPC stream, and
// go-platform's defaults write both logs and, with no OTLP endpoint, trace
// spans to stdout. Nothing here may let that happen: logs always go to stderr,
// and metrics-only mode never samples spans.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/jedi-knights/go-logging/pkg/logging"
	"github.com/jedi-knights/go-platform/httpserver"
	platformotel "github.com/jedi-knights/go-platform/otel"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/config"
)

// Mode says which telemetry signals are on.
type Mode int

// The modes. With neither an OTLP endpoint nor a metrics port configured the
// server is Inert, as sun-mcp's "unset means inert" requires.
const (
	// Inert installs no OpenTelemetry providers: spans and metrics are no-ops.
	Inert Mode = iota
	// MetricsOnly serves Prometheus metrics and records no spans.
	MetricsOnly
	// Full also exports traces over OTLP.
	Full
)

func (m Mode) String() string {
	switch m {
	case MetricsOnly:
		return "metrics-only"
	case Full:
		return "full"
	default:
		return "inert"
	}
}

// neverSample is go-platform's sampler ratio for "sample nothing": any ratio
// below zero. See metricsOnlyRatio.
const neverSample = -1.0

// Runtime is what Setup builds: the logger every component shares, the mode in
// effect, and the hook that flushes and stops telemetry.
type Runtime struct {
	Mode        Mode
	Logger      logging.Logger
	ServiceName string
	// Shutdown flushes buffered spans and stops the metrics listener. It is
	// safe to call more than once.
	Shutdown func(ctx context.Context) error
}

// modeOf derives the mode from the configuration.
func modeOf(cfg config.Config) Mode {
	switch {
	case cfg.OTLPEndpoint != "":
		return Full
	case cfg.MetricsPort > 0:
		return MetricsOnly
	default:
		return Inert
	}
}

// Setup builds the Runtime. It always installs a stderr JSON logger (made
// span-aware, so records carry trace_id and span_id) as the slog default; it
// installs OpenTelemetry providers only when the mode is not Inert. Call it
// once, before anything that emits telemetry, and defer Runtime.Shutdown.
func Setup(ctx context.Context, cfg config.Config, version string) (*Runtime, error) {
	handler := platformotel.NewSpanContextHandler(
		slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(slog.New(handler))

	rt := &Runtime{
		Mode:        modeOf(cfg),
		ServiceName: cfg.ServiceName,
		Logger:      logging.New(logging.Config{Handler: handler, ServiceName: cfg.ServiceName}),
		Shutdown:    func(context.Context) error { return nil },
	}
	if rt.Mode == Inert {
		return rt, nil
	}

	obs, err := platformotel.New(ctx, otelConfig(cfg, rt.Mode, version))
	if err != nil {
		return nil, fmt.Errorf("bootstrap: starting OpenTelemetry: %w", err)
	}
	stopMetrics := func(context.Context) error { return nil }
	if cfg.MetricsPort > 0 {
		ms, err := httpserver.StartMetricsServer(fmt.Sprintf(":%d", cfg.MetricsPort), "", obs.PromHandler)
		if err != nil {
			_ = obs.Shutdown(ctx) // best effort: the metrics listener failed, so give the providers back
			return nil, fmt.Errorf("bootstrap: starting the metrics listener on port %d: %w", cfg.MetricsPort, err)
		}
		stopMetrics = ms.Shutdown
	}
	rt.Shutdown = func(sctx context.Context) error {
		return errors.Join(stopMetrics(sctx), obs.Shutdown(sctx))
	}
	return rt, nil
}

// otelConfig builds go-platform's configuration. Logs are forced to stderr
// because its default is stdout.
func otelConfig(cfg config.Config, mode Mode, version string) platformotel.Config {
	oc := platformotel.Config{
		ServiceName:      cfg.ServiceName,
		ServiceVersion:   version,
		ExporterEndpoint: cfg.OTLPEndpoint,
		LogOutput:        os.Stderr,
		LogLevel:         cfg.LogLevel.String(),
	}
	if mode == MetricsOnly {
		// go-platform has no "no trace exporter" switch: with no endpoint it
		// installs a stdout exporter, which would corrupt a stdio stream.
		// Sampling nothing keeps that exporter silent. Its sampler is
		// parent-based, so WrapHTTP also stops extracting a caller's trace
		// context in this mode; otherwise a sampled traceparent would force
		// recording.
		oc.SamplerRatio = neverSample
	}
	return oc
}
