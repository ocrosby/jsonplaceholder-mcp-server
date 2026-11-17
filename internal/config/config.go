// Package config reads the server's environment-variable configuration.
// Names and defaults follow sun-mcp (MCP_TRANSPORT, HOST, PORT, LOG_LEVEL,
// API_HOST, METRICS_PORT) and the OpenTelemetry environment contract
// (OTEL_SERVICE_NAME, OTEL_EXPORTER_OTLP_*).
package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Transport is an MCP transport.
type Transport string

// The supported transports.
const (
	Stdio          Transport = "stdio"
	StreamableHTTP Transport = "streamable-http"
)

const (
	// EndpointPath is where the streamable-http transport serves MCP (same as sun-mcp).
	EndpointPath = "/mcp"
	// DefaultServiceName is the OpenTelemetry service.name when OTEL_SERVICE_NAME is unset.
	DefaultServiceName = "jsonplaceholder-mcp-server"
)

// Config is the validated server configuration.
type Config struct {
	Transport   Transport
	Host        string
	Port        int
	LogLevel    slog.Level
	APIHost     string // upstream base URL, no trailing slash
	HTTPTimeout time.Duration

	// ServiceName is the OpenTelemetry service.name.
	ServiceName string
	// MetricsPort serves Prometheus metrics on its own listener; 0 means no listener.
	MetricsPort int
	// OTLPEndpoint is where traces are exported; empty means no trace export.
	OTLPEndpoint string
}

// getter reads one setting: the trimmed value of key, or def when unset or blank.
type getter func(key, def string) string

// Load builds a Config from getenv (os.Getenv in production). Unset variables
// take their defaults; set-but-invalid ones are an error.
func Load(getenv func(string) string) (Config, error) {
	if getenv == nil {
		return Config{}, fmt.Errorf("config: getenv is required")
	}
	get := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}
	cfg := Config{Host: get("HOST", "0.0.0.0")}
	// Order matters: telemetry validation needs the transport and port.
	steps := []func(*Config, getter) error{applyTransport, applyPort, applyLogLevel, applyUpstream, applyTelemetry}
	for _, step := range steps {
		if err := step(&cfg, get); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

func applyTransport(cfg *Config, get getter) error {
	raw := get("MCP_TRANSPORT", string(Stdio))
	switch t := Transport(strings.ToLower(raw)); t {
	case Stdio, StreamableHTTP:
		cfg.Transport = t
		return nil
	default:
		return fmt.Errorf("config: MCP_TRANSPORT must be %q or %q, got %q", Stdio, StreamableHTTP, raw)
	}
}

func applyPort(cfg *Config, get getter) error {
	port, err := parseRange("PORT", get("PORT", "8000"), 1, 65535)
	cfg.Port = port
	return err
}

func applyLogLevel(cfg *Config, get getter) error {
	if err := cfg.LogLevel.UnmarshalText([]byte(get("LOG_LEVEL", "INFO"))); err != nil {
		return fmt.Errorf("config: LOG_LEVEL: %w", err)
	}
	return nil
}

func applyUpstream(cfg *Config, get getter) error {
	host, err := parseHTTPURL("API_HOST", get("API_HOST", "https://jsonplaceholder.typicode.com"))
	if err != nil {
		return err
	}
	cfg.APIHost = host
	secs, err := parseRange("HTTP_TIMEOUT_SECONDS", get("HTTP_TIMEOUT_SECONDS", "10"), 1, 120)
	cfg.HTTPTimeout = time.Duration(secs) * time.Second
	return err
}

// applyTelemetry reads the observability settings. All three are optional and,
// when none is set, telemetry stays inert (sun-mcp's "unset means inert").
func applyTelemetry(cfg *Config, get getter) error {
	cfg.ServiceName = get("OTEL_SERVICE_NAME", DefaultServiceName)

	endpoint, err := resolveOTLPEndpoint(get)
	if err != nil {
		return err
	}
	cfg.OTLPEndpoint = endpoint

	raw := get("METRICS_PORT", "")
	if raw == "" {
		return nil
	}
	port, err := parseRange("METRICS_PORT", raw, 1, 65535)
	if err != nil {
		return err
	}
	if cfg.Transport == StreamableHTTP && port == cfg.Port {
		return fmt.Errorf("config: METRICS_PORT must differ from PORT (%d): the scrape listener is separate from the MCP listener", port)
	}
	cfg.MetricsPort = port
	return nil
}

// tracesPath is the OTLP/HTTP signal path appended to a base endpoint.
const tracesPath = "/v1/traces"

// resolveOTLPEndpoint returns the complete URL traces are exported to, or ""
// when no endpoint is configured. It follows the OpenTelemetry spec because the
// exporter (since v1.45) takes the URL as-is: OTEL_EXPORTER_OTLP_TRACES_ENDPOINT
// is already complete, while the generic OTEL_EXPORTER_OTLP_ENDPOINT is a base
// URL to which OTLP/HTTP appends /v1/traces. gRPC needs only host and port.
func resolveOTLPEndpoint(get getter) (string, error) {
	if specific := get("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", ""); specific != "" {
		return parseHTTPURL("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", specific)
	}
	base := get("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	if base == "" {
		return "", nil
	}
	endpoint, err := parseHTTPURL("OTEL_EXPORTER_OTLP_ENDPOINT", base)
	if err != nil {
		return "", err
	}
	protocol := strings.ToLower(get("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", get("OTEL_EXPORTER_OTLP_PROTOCOL", "grpc")))
	if strings.HasPrefix(protocol, "http") {
		endpoint += tracesPath
	}
	return endpoint, nil
}

// parseRange parses an integer env value that must lie in [lo, hi].
func parseRange(name, raw string, lo, hi int) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil || n < lo || n > hi {
		return 0, fmt.Errorf("config: %s must be %d-%d, got %q", name, lo, hi, raw)
	}
	return n, nil
}

// parseHTTPURL validates an http(s) URL and returns it without a trailing slash.
func parseHTTPURL(name, raw string) (string, error) {
	trimmed := strings.TrimRight(raw, "/")
	u, err := url.Parse(trimmed)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("config: %s must be an http(s) URL, got %q", name, raw)
	}
	return trimmed, nil
}
