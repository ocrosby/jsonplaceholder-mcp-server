package bootstrap_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/bootstrap"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/config"
)

// These tests install process-wide OpenTelemetry globals, so none are parallel.

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

// resetGlobals restores the process-wide state Setup mutates.
func resetGlobals(t *testing.T) {
	t.Helper()
	prevLogger := slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(prevLogger)
		otel.SetTracerProvider(noop.NewTracerProvider())
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
	})
}

func base() config.Config {
	return config.Config{ServiceName: "svc-test", LogLevel: slog.LevelInfo}
}

func TestNoTelemetryConfigIsInertAndLeavesTheGlobalsAlone(t *testing.T) {
	resetGlobals(t)
	before := otel.GetTracerProvider()
	rt, err := bootstrap.Setup(context.Background(), base(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if rt.Mode != bootstrap.Inert || rt.Logger == nil {
		t.Errorf("mode = %v, logger = %v", rt.Mode, rt.Logger)
	}
	if otel.GetTracerProvider() != before {
		t.Error("an inert setup must not install a tracer provider")
	}
	if err := rt.Shutdown(context.Background()); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}

func TestMetricsOnlyServesPrometheusAndNeverSamplesSpans(t *testing.T) {
	resetGlobals(t)
	cfg := base()
	cfg.MetricsPort = freePort(t)
	rt, err := bootstrap.Setup(context.Background(), cfg, "test")
	if err != nil {
		t.Fatal(err)
	}
	if rt.Mode != bootstrap.MetricsOnly {
		t.Fatalf("mode = %v", rt.Mode)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/metrics", cfg.MetricsPort)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "go_goroutines") {
		t.Errorf("GET /metrics -> %d, body lacks go_goroutines", resp.StatusCode)
	}
	// go-platform's trace exporter defaults to stdout, which would corrupt a
	// stdio MCP stream; metrics-only mode must never record, so never export.
	_, span := otel.Tracer("t").Start(context.Background(), "must-not-record")
	if span.IsRecording() {
		t.Error("metrics-only mode must not sample spans")
	}
	span.End()

	if err := rt.Shutdown(context.Background()); err != nil {
		t.Errorf("shutdown: %v", err)
	}
	if resp, err := http.Get(url); err == nil {
		_ = resp.Body.Close()
		t.Error("the metrics listener is still up after Shutdown")
	}
}

func TestFullModeExportsSpansOverOTLP(t *testing.T) {
	resetGlobals(t)
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
	var mu sync.Mutex
	var paths []string
	var bodies strings.Builder
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		paths = append(paths, r.URL.Path)
		bodies.Write(b)
		mu.Unlock()
	}))
	defer collector.Close()

	cfg := base()
	cfg.OTLPEndpoint = collector.URL + "/v1/traces" // config.Load resolves the complete URL; the exporter uses it as-is
	rt, err := bootstrap.Setup(context.Background(), cfg, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if rt.Mode != bootstrap.Full {
		t.Fatalf("mode = %v", rt.Mode)
	}
	_, span := otel.Tracer("t").Start(context.Background(), "exported-span")
	span.End()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rt.Shutdown(ctx); err != nil { // flushes the batch processor
		t.Fatalf("shutdown: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(paths) == 0 || paths[0] != "/v1/traces" || !strings.Contains(bodies.String(), "exported-span") || !strings.Contains(bodies.String(), "svc-test") {
		t.Errorf("collector saw paths %v; body has span=%v service=%v", paths,
			strings.Contains(bodies.String(), "exported-span"), strings.Contains(bodies.String(), "svc-test"))
	}
}

func TestWrapHTTPContinuesTheCallersTraceAndSkipsProbes(t *testing.T) {
	resetGlobals(t)
	spans := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans)))
	otel.SetTextMapPropagator(propagation.TraceContext{})

	rt, err := bootstrap.Setup(context.Background(), base(), "test") // inert: only the logger is used
	if err != nil {
		t.Fatal(err)
	}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	h := bootstrap.WrapHTTP(ok, rt)

	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("traceparent", "00-"+traceID+"-00f067aa0ba902b7-01")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Trace-ID"); got != traceID {
		t.Errorf("X-Trace-ID = %q, want the caller's %s", got, traceID)
	}
	ended := spans.Ended()
	if len(ended) != 1 || ended[0].SpanContext().TraceID().String() != traceID || ended[0].Parent().SpanID().String() != "00f067aa0ba902b7" {
		t.Fatalf("server span = %v, want one child of the caller's span in trace %s", ended, traceID)
	}

	for _, probe := range []string{"/livez", "/readyz", "/health"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", probe, nil))
	}
	if n := len(spans.Ended()); n != 1 {
		t.Errorf("%d spans after probe requests, want still 1 (probes must not be traced)", n)
	}
}

// Metrics-only mode promises "no spans". go-platform's sampler is parent-based,
// so a caller's sampled traceparent would otherwise force recording, and its
// default trace exporter would print those spans to stdout.
func TestMetricsOnlyIgnoresACallersSampledTraceContext(t *testing.T) {
	resetGlobals(t)
	cfg := base()
	cfg.MetricsPort = freePort(t)
	rt, err := bootstrap.Setup(context.Background(), cfg, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })

	var recording bool
	probe := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		recording = trace.SpanFromContext(r.Context()).IsRecording()
	})
	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01") // sampled
	bootstrap.WrapHTTP(probe, rt).ServeHTTP(httptest.NewRecorder(), req)

	if recording {
		t.Error("a sampled traceparent made a metrics-only server record spans")
	}
}
