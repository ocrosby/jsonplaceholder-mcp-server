package main_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// These tests run the real binary. They guard the failure modes only a real
// process shows: stdout corruption under stdio, and graceful shutdown.

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "jpmcp-e2e")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "server")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

func fakeUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/posts/1" {
			_, _ = w.Write([]byte(`{"id":1,"userId":1,"title":"hello","body":"world"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(ts.Close)
	return ts
}

// collector is a fake OTLP/HTTP endpoint that remembers every body it receives.
type collector struct {
	*httptest.Server
	mu    sync.Mutex
	body  strings.Builder
	paths map[string]int
}

func newCollector(t *testing.T) *collector {
	t.Helper()
	c := &collector{paths: map[string]int{}}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.body.Write(b)
		c.paths[r.URL.Path]++
		c.mu.Unlock()
	}))
	t.Cleanup(c.Close)
	return c
}

// received reports how many requests arrived at path.
func (c *collector) received(path string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.paths[path]
}

func (c *collector) saw(s string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.Contains(c.body.String(), s)
}

func TestStdioStdoutStaysProtocolOnlyInEveryTelemetryMode(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the real binary")
	}
	// telemetryMode is one way to configure telemetry, plus the fake collector
	// to inspect afterwards when the mode exports traces.
	type telemetryMode struct {
		env []string
		col *collector
	}
	modes := map[string]func(t *testing.T) telemetryMode{
		"inert": func(*testing.T) telemetryMode { return telemetryMode{} },
		"metrics only": func(t *testing.T) telemetryMode {
			return telemetryMode{env: []string{fmt.Sprintf("METRICS_PORT=%d", freePort(t))}}
		},
		"full": func(t *testing.T) telemetryMode {
			c := newCollector(t)
			return telemetryMode{env: []string{"OTEL_EXPORTER_OTLP_ENDPOINT=" + c.URL, "OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf"}, col: c}
		},
	}
	for name, mode := range modes {
		t.Run(name, func(t *testing.T) {
			upstream := fakeUpstream(t)
			m := mode(t)
			col := m.col
			cmd := exec.Command(binary)
			cmd.Env = append([]string{"MCP_TRANSPORT=stdio", "API_HOST=" + upstream.URL, "LOG_LEVEL=DEBUG"}, m.env...)
			stdin, _ := cmd.StdinPipe()
			stdout, _ := cmd.StdoutPipe()
			var stderr strings.Builder
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cmd.Process.Kill() })

			for _, req := range []string{
				`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`,
				`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
				`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_post","arguments":{"id":1}}}`,
			} {
				_, _ = io.WriteString(stdin, req+"\n")
			}

			lines := make(chan string, 64)
			go func() {
				sc := bufio.NewScanner(stdout)
				sc.Buffer(make([]byte, 1<<20), 1<<20)
				for sc.Scan() {
					lines <- sc.Text()
				}
				close(lines)
			}()
			var got []string
			gotToolResult := false
			deadline := time.After(15 * time.Second)
			for !gotToolResult {
				select {
				case l, ok := <-lines:
					if !ok {
						t.Fatalf("stdout closed before the tool result; stderr:\n%s", stderr.String())
					}
					got = append(got, l)
					gotToolResult = strings.Contains(l, `"id":2`)
				case <-deadline:
					t.Fatalf("no tool result in 15s; stdout so far: %q\nstderr:\n%s", got, stderr.String())
				}
			}
			_ = stdin.Close()
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("exit: %v\nstderr:\n%s", err, stderr.String())
				}
			case <-time.After(15 * time.Second):
				t.Fatal("process did not exit after stdin closed")
			}
			for l := range lines {
				got = append(got, l)
			}

			for _, l := range got {
				var msg map[string]any
				if err := json.Unmarshal([]byte(l), &msg); err != nil || msg["jsonrpc"] != "2.0" {
					t.Fatalf("stdout line is not JSON-RPC (telemetry leaked onto the protocol stream): %.200q", l)
				}
			}
			if !strings.Contains(strings.Join(got, "\n"), `"isError":false`) && strings.Contains(strings.Join(got, "\n"), `"isError":true`) {
				t.Errorf("tool call failed: %v", got)
			}
			if col != nil {
				if col.received("/v1/traces") == 0 {
					t.Errorf("no export reached /v1/traces; paths seen: %v", col.paths)
				}
				for _, want := range []string{"tool.invoke", "jsonplaceholder.posts.get"} {
					if !col.saw(want) {
						t.Errorf("collector never received a span named %q", want)
					}
				}
			}
		})
	}
}

func TestHTTPServesMCPWithTracesMetricsAndGracefulShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the real binary")
	}
	upstream := fakeUpstream(t)
	col := newCollector(t)
	port, metricsPort := freePort(t), freePort(t)
	cmd := exec.Command(binary)
	cmd.Env = []string{
		"MCP_TRANSPORT=streamable-http", "HOST=127.0.0.1", fmt.Sprintf("PORT=%d", port),
		fmt.Sprintf("METRICS_PORT=%d", metricsPort), "API_HOST=" + upstream.URL,
		"OTEL_EXPORTER_OTLP_ENDPOINT=" + col.URL, "OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf",
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitFor(t, base+"/livez", &stderr)

	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	req, _ := http.NewRequest("POST", base+"/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_post","arguments":{"id":1}}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("traceparent", "00-"+traceID+"-00f067aa0ba902b7-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "hello") {
		t.Fatalf("tools/call -> %d %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("X-Trace-ID"); got != traceID {
		t.Errorf("X-Trace-ID = %q, want the caller's trace %s", got, traceID)
	}

	mresp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/metrics", metricsPort))
	if err != nil {
		t.Fatal(err)
	}
	mb, _ := io.ReadAll(mresp.Body)
	_ = mresp.Body.Close()
	for _, want := range []string{"mcp_tool_invocations_total", "jsonplaceholder_client_operation_duration_seconds_count", "http_server_request_duration", "go_goroutines"} {
		if !strings.Contains(string(mb), want) {
			t.Errorf("/metrics lacks %s", want)
		}
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("did not exit cleanly on SIGTERM: %v\nstderr:\n%s", err, stderr.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("no exit within 15s of SIGTERM")
	}
	// OTLP carries trace ids as raw bytes, so the span names are the checkable
	// evidence that spans were flushed on shutdown.
	if col.received("/v1/traces") == 0 {
		t.Errorf("no export reached /v1/traces; paths seen: %v", col.paths)
	}
	for _, want := range []string{"tool.invoke", "jsonplaceholder.posts.get"} {
		if !col.saw(want) {
			t.Errorf("collector never received %q (spans must be flushed on shutdown)", want)
		}
	}
}

func waitFor(t *testing.T, url string, stderr *strings.Builder) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := http.Get(url); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s never came up; stderr:\n%s", url, stderr.String())
}
