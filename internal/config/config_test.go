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

func TestInvalid(t *testing.T) {
	bad := map[string]map[string]string{
		"transport":    {"MCP_TRANSPORT": "sse"},
		"port zero":    {"PORT": "0"},
		"port high":    {"PORT": "65536"},
		"port text":    {"PORT": "abc"},
		"log level":    {"LOG_LEVEL": "loud"},
		"api host":     {"API_HOST": "ftp://x"},
		"timeout zero": {"HTTP_TIMEOUT_SECONDS": "0"},
		"timeout high": {"HTTP_TIMEOUT_SECONDS": "121"},
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
