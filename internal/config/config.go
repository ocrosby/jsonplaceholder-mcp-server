// Package config reads the server's environment-variable configuration.
// Names and defaults follow sun-mcp (MCP_TRANSPORT, HOST, PORT, LOG_LEVEL, API_HOST).
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

// EndpointPath is where the streamable-http transport serves MCP (same as sun-mcp).
const EndpointPath = "/mcp"

// Config is the validated server configuration.
type Config struct {
	Transport   Transport
	Host        string
	Port        int
	LogLevel    slog.Level
	APIHost     string // upstream base URL, no trailing slash
	HTTPTimeout time.Duration
}

// Load builds a Config from getenv (os.Getenv in production). Unset variables
// take their defaults; set-but-invalid ones are an error.
func Load(getenv func(string) string) (Config, error) {
	if getenv == nil {
		return Config{}, fmt.Errorf("config: getenv is required")
	}
	get := func(key, def string) string { return envOr(getenv, key, def) }
	cfg := Config{Host: get("HOST", "0.0.0.0")}
	var err error
	if cfg.Transport, err = parseTransport(get("MCP_TRANSPORT", string(Stdio))); err != nil {
		return Config{}, err
	}
	if cfg.Port, err = parseRange("PORT", get("PORT", "8000"), 1, 65535); err != nil {
		return Config{}, err
	}
	if cfg.LogLevel, err = parseLogLevel(get("LOG_LEVEL", "INFO")); err != nil {
		return Config{}, err
	}
	if cfg.APIHost, err = parseAPIHost(get("API_HOST", "https://jsonplaceholder.typicode.com")); err != nil {
		return Config{}, err
	}
	secs, err := parseRange("HTTP_TIMEOUT_SECONDS", get("HTTP_TIMEOUT_SECONDS", "10"), 1, 120)
	if err != nil {
		return Config{}, err
	}
	cfg.HTTPTimeout = time.Duration(secs) * time.Second
	return cfg, nil
}

func parseTransport(raw string) (Transport, error) {
	switch t := Transport(strings.ToLower(raw)); t {
	case Stdio, StreamableHTTP:
		return t, nil
	default:
		return "", fmt.Errorf("config: MCP_TRANSPORT must be %q or %q, got %q", Stdio, StreamableHTTP, raw)
	}
}

// parseRange parses an integer env value that must lie in [lo, hi].
func parseRange(name, raw string, lo, hi int) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil || n < lo || n > hi {
		return 0, fmt.Errorf("config: %s must be %d-%d, got %q", name, lo, hi, raw)
	}
	return n, nil
}

func parseAPIHost(raw string) (string, error) {
	host := strings.TrimRight(raw, "/")
	u, err := url.Parse(host)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("config: API_HOST must be an http(s) URL, got %q", raw)
	}
	return host, nil
}

func parseLogLevel(raw string) (slog.Level, error) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(raw)); err != nil {
		return 0, fmt.Errorf("config: LOG_LEVEL: %w", err)
	}
	return l, nil
}

// envOr returns the trimmed value of key, or def when it is unset or blank.
func envOr(getenv func(string) string, key, def string) string {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		return v
	}
	return def
}
