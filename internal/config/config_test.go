package config_test

import (
	"log/slog"
	"testing"
	"time"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/config"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestDefaults(t *testing.T) {
	cfg, err := config.Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	want := config.Config{
		Transport: config.Stdio, Host: "0.0.0.0", Port: 8000, LogLevel: slog.LevelInfo,
		APIHost: "https://jsonplaceholder.typicode.com", HTTPTimeout: 10 * time.Second,
		ServiceName: "jsonplaceholder-mcp-server",
	}
	if cfg != want {
		t.Errorf("got %+v, want %+v", cfg, want)
	}
}

func TestOverrides(t *testing.T) {
	cfg, err := config.Load(env(map[string]string{
		"MCP_TRANSPORT": "Streamable-HTTP", "PORT": "9090", "LOG_LEVEL": "debug",
		"API_HOST": "http://localhost:3000/", "HTTP_TIMEOUT_SECONDS": "3",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Transport != config.StreamableHTTP || cfg.Port != 9090 || cfg.LogLevel != slog.LevelDebug ||
		cfg.APIHost != "http://localhost:3000" || cfg.HTTPTimeout != 3*time.Second {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

func TestTelemetryEnv(t *testing.T) {
	cfg, err := config.Load(env(map[string]string{
		"OTEL_SERVICE_NAME": "svc-x", "METRICS_PORT": "9464", "OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4318",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServiceName != "svc-x" || cfg.MetricsPort != 9464 || cfg.OTLPEndpoint != "http://collector:4318" {
		t.Errorf("unexpected telemetry config: %+v", cfg)
	}
}

func TestTracesEndpointWinsOverTheGenericOne(t *testing.T) {
	cfg, err := config.Load(env(map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://generic:4318", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "http://traces:4318",
	}))
	if err != nil || cfg.OTLPEndpoint != "http://traces:4318" {
		t.Errorf("endpoint = %q, err = %v", cfg.OTLPEndpoint, err)
	}
}

// The OpenTelemetry spec treats OTEL_EXPORTER_OTLP_ENDPOINT as a base URL (OTLP/HTTP appends the
// signal path) and OTEL_EXPORTER_OTLP_TRACES_ENDPOINT as the complete URL. The exporter takes the
// final URL as-is, so Load must resolve it.
func TestOTLPEndpointResolutionFollowsTheSpec(t *testing.T) {
	cases := map[string]struct {
		env  map[string]string
		want string
	}{
		"generic over gRPC is left alone": {
			map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://c:4317"}, "http://c:4317"},
		"generic over http appends the signal path": {
			map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://c:4318", "OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf"},
			"http://c:4318/v1/traces"},
		"generic with a base path keeps it": {
			map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "https://gw.example/otlp/", "OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf"},
			"https://gw.example/otlp/v1/traces"},
		"the traces-specific protocol decides": {
			map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://c:4318", "OTEL_EXPORTER_OTLP_PROTOCOL": "grpc", "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL": "http/protobuf"},
			"http://c:4318/v1/traces"},
		"traces endpoint is used as-is even over http": {
			map[string]string{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "http://c:4318/custom", "OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf"},
			"http://c:4318/custom"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg, err := config.Load(env(tc.env))
			if err != nil || cfg.OTLPEndpoint != tc.want {
				t.Errorf("OTLPEndpoint = %q (err %v), want %q", cfg.OTLPEndpoint, err, tc.want)
			}
		})
	}
}

func TestMetricsPortMayEqualPortOnStdio(t *testing.T) {
	// PORT is unused under stdio, so sharing the number is harmless there.
	if _, err := config.Load(env(map[string]string{"PORT": "9000", "METRICS_PORT": "9000"})); err != nil {
		t.Errorf("stdio must not reject METRICS_PORT == PORT: %v", err)
	}
}

func TestInvalid(t *testing.T) {
	bad := map[string]map[string]string{
		"transport":          {"MCP_TRANSPORT": "sse"},
		"port zero":          {"PORT": "0"},
		"port high":          {"PORT": "65536"},
		"port text":          {"PORT": "abc"},
		"log level":          {"LOG_LEVEL": "loud"},
		"api host":           {"API_HOST": "ftp://x"},
		"timeout zero":       {"HTTP_TIMEOUT_SECONDS": "0"},
		"timeout high":       {"HTTP_TIMEOUT_SECONDS": "121"},
		"metrics port zero":  {"METRICS_PORT": "0"},
		"metrics port text":  {"METRICS_PORT": "abc"},
		"metrics port high":  {"METRICS_PORT": "65536"},
		"metrics port clash": {"MCP_TRANSPORT": "streamable-http", "PORT": "9000", "METRICS_PORT": "9000"},
		"otlp endpoint":      {"OTEL_EXPORTER_OTLP_ENDPOINT": "collector:4318"},
	}
	for name, m := range bad {
		t.Run(name, func(t *testing.T) {
			if _, err := config.Load(env(m)); err == nil {
				t.Error("expected error")
			}
		})
	}
	if _, err := config.Load(nil); err == nil {
		t.Error("nil getenv must error")
	}
}

func TestCompactResults(t *testing.T) {
	for in, want := range map[string]bool{"": false, "false": false, "true": true, "1": true} {
		cfg, err := config.Load(env(map[string]string{"COMPACT_RESULTS": in}))
		if err != nil || cfg.CompactResults != want {
			t.Errorf("COMPACT_RESULTS=%q: got %v, %v; want %v", in, cfg.CompactResults, err, want)
		}
	}
	if _, err := config.Load(env(map[string]string{"COMPACT_RESULTS": "maybe"})); err == nil {
		t.Error("COMPACT_RESULTS=maybe: expected error")
	}
}
