package mcpserver_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/adapters/inbound/mcpserver"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/application"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/domain"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/ports"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/requestlog"
)

// fakeSource serves one post and a failing/empty remainder; unset methods panic via the nil embed.
type fakeSource struct {
	ports.ResourceSource
	todoFilter domain.TodoFilter
	failStatus int
}

func (f *fakeSource) GetPost(ctx context.Context, id int) (domain.Post, error) {
	requestlog.Record(ctx, "GET https://example.test/posts/"+itoa(id))
	switch {
	case f.failStatus != 0:
		return domain.Post{}, domain.Upstream(f.failStatus, "upstream returned HTTP %d", f.failStatus)
	case id == 1:
		return domain.Post{ID: 1, UserID: 1, Title: "hello", Body: "world"}, nil
	}
	return domain.Post{}, domain.NotFound("post %d not found", id)
}

func (f *fakeSource) ListTodos(_ context.Context, fl domain.TodoFilter, _ domain.ListOptions) ([]domain.Todo, error) {
	f.todoFilter = fl
	return []domain.Todo{{ID: 9, UserID: 2, Title: "buy milk", Completed: true}}, nil
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func newClient(t *testing.T, src *fakeSource) *client.Client {
	t.Helper()
	return newClientWith(t, src)
}

func newClientWith(t *testing.T, src *fakeSource, opts ...mcpserver.Option) *client.Client {
	t.Helper()
	svc, err := application.NewService(src)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := mcpserver.New(svc, "test", opts...)
	if err != nil {
		t.Fatal(err)
	}
	c, err := client.NewInProcessClient(srv)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Initialize(ctx, mcp.InitializeRequest{}); err != nil {
		t.Fatal(err)
	}
	return c
}

func callTool(t *testing.T, c *client.Client, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name, req.Params.Arguments = name, args
	res, err := c.CallTool(context.Background(), req)
	if err != nil {
		t.Fatalf("%s: protocol error: %v", name, err)
	}
	return res
}

func structured(t *testing.T, res *mcp.CallToolResult) (result map[string]any, apiRequests []any) {
	t.Helper()
	m, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %#v", res.StructuredContent)
	}
	return m["result"].(map[string]any), m["api_requests"].([]any)
}

func text(res *mcp.CallToolResult) string { return res.Content[0].(mcp.TextContent).Text }

func TestListToolsAreAllReadOnly(t *testing.T) {
	c := newClient(t, &fakeSource{})
	out, err := c.ListTools(context.Background(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]mcp.Tool{}
	for _, tl := range out.Tools {
		got[tl.Name] = tl
	}
	if len(got) != len(mcpserver.ToolNames) {
		t.Errorf("%d tools registered, want %d", len(got), len(mcpserver.ToolNames))
	}
	for _, name := range mcpserver.ToolNames {
		tl, ok := got[name]
		if !ok {
			t.Errorf("tool %q missing", name)
			continue
		}
		a := tl.Annotations
		if a.ReadOnlyHint == nil || !*a.ReadOnlyHint || a.DestructiveHint == nil || *a.DestructiveHint ||
			a.IdempotentHint == nil || !*a.IdempotentHint || a.OpenWorldHint == nil || !*a.OpenWorldHint {
			t.Errorf("tool %q annotations = %+v", name, a)
		}
		if tl.Description == "" {
			t.Errorf("tool %q has no description", name)
		}
	}
}

func TestInitializeCarriesInstructions(t *testing.T) {
	svc, _ := application.NewService(&fakeSource{})
	srv, _ := mcpserver.New(svc, "1.2.3")
	c, _ := client.NewInProcessClient(srv)
	defer func() { _ = c.Close() }()
	_ = c.Start(context.Background())
	res, err := c.Initialize(context.Background(), mcp.InitializeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if res.ServerInfo.Name != mcpserver.ServerName || res.ServerInfo.Version != "1.2.3" || !strings.Contains(res.Instructions, "list_posts") {
		t.Errorf("serverInfo %+v instructions %q", res.ServerInfo, res.Instructions)
	}
}

func TestSuccessResultShape(t *testing.T) {
	res := callTool(t, newClient(t, &fakeSource{}), "get_post", map[string]any{"id": 1})
	if res.IsError {
		t.Fatalf("isError: %s", text(res))
	}
	result, reqs := structured(t, res)
	if result["status_code"] != float64(200) || result["payload"].(map[string]any)["title"] != "hello" {
		t.Errorf("result = %#v", result)
	}
	if prose := result["prose"].(string); !strings.Contains(prose, "hello") || strings.Contains(prose, "API Requests") {
		t.Errorf("prose = %q", prose)
	}
	if len(reqs) != 1 || !strings.HasSuffix(reqs[0].(string), "/posts/1") {
		t.Errorf("api_requests = %v", reqs)
	}
	if !strings.Contains(text(res), "API Requests:\n  GET ") {
		t.Errorf("content lacks the API Requests footer: %q", text(res))
	}
}

func TestFailuresAreIsErrorResults(t *testing.T) {
	cases := []struct {
		name   string
		src    *fakeSource
		args   map[string]any
		status any // JSON value of status_code
		prose  string
	}{
		{"not found", &fakeSource{}, map[string]any{"id": 5}, float64(404), "Not found:"},
		{"upstream", &fakeSource{failStatus: 503}, map[string]any{"id": 1}, float64(503), "Upstream error:"},
		{"invalid id", &fakeSource{}, map[string]any{"id": 0}, nil, "Invalid request:"},
		{"missing id", &fakeSource{}, map[string]any{}, nil, "Invalid request: id is required"},
		{"fractional id", &fakeSource{}, map[string]any{"id": 1.5}, nil, "Invalid request:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := callTool(t, newClient(t, tc.src), "get_post", tc.args)
			if !res.IsError {
				t.Fatal("isError must be true")
			}
			result, _ := structured(t, res)
			if result["status_code"] != tc.status {
				t.Errorf("status_code = %v, want %v", result["status_code"], tc.status)
			}
			if !strings.Contains(result["prose"].(string), tc.prose) {
				t.Errorf("prose = %q, want containing %q", result["prose"], tc.prose)
			}
		})
	}
}

func TestListArgumentsReachTheService(t *testing.T) {
	src := &fakeSource{}
	res := callTool(t, newClient(t, src), "list_todos", map[string]any{"user_id": 2, "completed": true})
	if res.IsError {
		t.Fatal(text(res))
	}
	if src.todoFilter.UserID == nil || *src.todoFilter.UserID != 2 || src.todoFilter.Completed == nil || !*src.todoFilter.Completed {
		t.Errorf("filter = %+v", src.todoFilter)
	}
	if !strings.Contains(text(res), "[x] buy milk") {
		t.Errorf("text = %q", text(res))
	}
	if bad := callTool(t, newClient(t, src), "list_todos", map[string]any{"completed": "yes"}); !bad.IsError {
		t.Error("non-boolean completed must be rejected")
	}
	if bad := callTool(t, newClient(t, src), "list_todos", map[string]any{"limit": 101}); !bad.IsError {
		t.Error("limit above the cap must be rejected")
	}
}

func TestHTTPTransportIsStatelessAndServesProbes(t *testing.T) {
	svc, _ := application.NewService(&fakeSource{})
	srv, _ := mcpserver.New(svc, "test")
	ts := httptest.NewServer(mcpserver.NewHTTPHandler(srv, "/mcp"))
	t.Cleanup(ts.Close)

	for _, p := range []string{"/livez", "/readyz", "/health"} {
		resp, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(string(body), `"ok"`) {
			t.Errorf("%s -> %d %s", p, resp.StatusCode, body)
		}
	}

	// No initialize handshake and no session header: stateless mode must still answer tools/list.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(string(body), "list_photos") {
		t.Errorf("tools/list over HTTP -> %d %s", resp.StatusCode, body)
	}
}

func TestEveryToolCarriesDiscoveryMeta(t *testing.T) {
	c := newClient(t, &fakeSource{})
	out, err := c.ListTools(context.Background(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range out.Tools {
		if tl.Meta == nil {
			t.Errorf("tool %q has no _meta", tl.Name)
			continue
		}
		m, ok := tl.Meta.AdditionalFields[mcpserver.MetaKey].(map[string]any)
		if !ok {
			t.Errorf("tool %q: _meta[%q] = %#v", tl.Name, mcpserver.MetaKey, tl.Meta.AdditionalFields)
			continue
		}
		op, _ := m["operation"].(string)
		resource, _ := m["resource"].(string)
		if op != "list" && op != "get" {
			t.Errorf("tool %q: operation = %q", tl.Name, op)
		}
		if !strings.HasPrefix(tl.Name, op+"_") {
			t.Errorf("tool %q does not match its operation %q", tl.Name, op)
		}
		if paged, _ := m["paged"].(bool); paged != (op == "list") {
			t.Errorf("tool %q: paged = %v for operation %q", tl.Name, paged, op)
		}
		if !strings.Contains(strings.ToLower(tl.Description), strings.TrimSuffix(resource, "s")) {
			t.Errorf("tool %q: description never mentions %q", tl.Name, resource)
		}
	}
}

// Compact mode drops the raw payload from structuredContent: the prose already
// carries the same data, and sending both roughly doubles every result.
func TestCompactResultsDropThePayloadOnly(t *testing.T) {
	full := callTool(t, newClient(t, &fakeSource{}), "get_post", map[string]any{"id": 1})
	res := callTool(t, newClientWith(t, &fakeSource{}, mcpserver.WithCompactResults(true)), "get_post", map[string]any{"id": 1})
	if res.IsError {
		t.Fatalf("isError: %s", text(res))
	}
	result, reqs := structured(t, res)
	if _, has := result["payload"]; has {
		t.Errorf("compact result still carries payload: %#v", result)
	}
	fullResult, _ := structured(t, full)
	if result["prose"] != fullResult["prose"] || result["status_code"] != float64(200) || len(reqs) != 1 {
		t.Errorf("compact result lost prose, status or api_requests: %#v %v", result, reqs)
	}
	if text(res) != text(full) {
		t.Errorf("content differs from the full result: %q vs %q", text(res), text(full))
	}
}

func TestCompactFailuresStayIsErrorWithStatus(t *testing.T) {
	c := newClientWith(t, &fakeSource{}, mcpserver.WithCompactResults(true))
	res := callTool(t, c, "get_post", map[string]any{"id": 0})
	if !res.IsError {
		t.Fatalf("expected isError, got %q", text(res))
	}
	if result, _ := structured(t, res); result["prose"] == "" {
		t.Errorf("failure lost its prose: %#v", result)
	}
}
