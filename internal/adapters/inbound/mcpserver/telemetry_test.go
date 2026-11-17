package mcpserver_test

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/adapters/inbound/mcpserver"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/application"
)

type toolTelemetry struct {
	client *client.Client
	spans  *tracetest.SpanRecorder
	reader *sdkmetric.ManualReader
}

func newToolTelemetry(t *testing.T) *toolTelemetry {
	t.Helper()
	spans := tracetest.NewSpanRecorder()
	reader := sdkmetric.NewManualReader()
	svc, err := application.NewService(&fakeSource{})
	if err != nil {
		t.Fatal(err)
	}
	srv, err := mcpserver.New(svc, "test",
		mcpserver.WithTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))),
		mcpserver.WithMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))))
	if err != nil {
		t.Fatal(err)
	}
	c, err := client.NewInProcessClient(srv)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if err := c.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Initialize(context.Background(), mcp.InitializeRequest{}); err != nil {
		t.Fatal(err)
	}
	return &toolTelemetry{client: c, spans: spans, reader: reader}
}

func (tt *toolTelemetry) toolSpan(t *testing.T) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, s := range tt.spans.Ended() {
		if s.Name() == "tool.invoke" {
			return s
		}
	}
	t.Fatal("no tool.invoke span")
	return nil
}

func spanAttr(s sdktrace.ReadOnlySpan, key string) (string, bool) {
	for _, kv := range s.Attributes() {
		if string(kv.Key) == key {
			return kv.Value.String(), true
		}
	}
	return "", false
}

func (tt *toolTelemetry) metric(t *testing.T, name string) metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := tt.reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m
			}
		}
	}
	t.Fatalf("metric %q missing", name)
	return metricdata.Metrics{}
}

func pointAttr(set attribute.Set, key string) string {
	v, _ := set.Value(attribute.Key(key))
	return v.AsString()
}

func TestToolCallIsASpanWithThePortfolioVocabulary(t *testing.T) {
	tt := newToolTelemetry(t)
	if res := callTool(t, tt.client, "get_post", map[string]any{"id": 1}); res.IsError {
		t.Fatal(text(res))
	}
	s := tt.toolSpan(t)
	want := map[string]string{
		"tool.name":       "get_post",
		"action":          "invoke",
		"resource":        "tool:get_post",
		"resource.kind":   "tool",
		"resource.id":     "get_post",
		"resource.parent": "jsonplaceholder-mcp-server",
		"resource.path":   "jsonplaceholder-mcp-server/tool/get_post",
	}
	for k, v := range want {
		if got, ok := spanAttr(s, k); !ok || got != v {
			t.Errorf("attribute %s = %q (present=%v), want %q", k, got, ok, v)
		}
	}
	if s.Status().Code == codes.Error {
		t.Errorf("status = %v", s.Status())
	}
	if len(s.Attributes()) != len(want) {
		t.Errorf("attributes = %v: only the vocabulary above may be recorded, never the call's arguments", s.Attributes())
	}
}

func TestToolFailuresAreMarkedWithTheErrorType(t *testing.T) {
	cases := []struct {
		name      string
		args      map[string]any
		errorType string
	}{
		{"not found", map[string]any{"id": 5}, "404"},
		{"invalid request", map[string]any{"id": 0}, "invalid_request"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tt := newToolTelemetry(t)
			if res := callTool(t, tt.client, "get_post", tc.args); !res.IsError {
				t.Fatal("expected a tool failure")
			}
			s := tt.toolSpan(t)
			if s.Status().Code != codes.Error {
				t.Errorf("status = %v, want Error", s.Status())
			}
			if got, _ := spanAttr(s, "error.type"); got != tc.errorType {
				t.Errorf("error.type = %q, want %q", got, tc.errorType)
			}
			inv, ok := tt.metric(t, "mcp.tool.invocations").Data.(metricdata.Sum[int64])
			if !ok || len(inv.DataPoints) != 1 || inv.DataPoints[0].Value != 1 {
				t.Fatalf("invocations = %+v", inv)
			}
			a := inv.DataPoints[0].Attributes
			if pointAttr(a, "tool.name") != "get_post" || pointAttr(a, "result") != "error" || pointAttr(a, "error.type") != tc.errorType {
				t.Errorf("invocation attributes = %v", a.ToSlice())
			}
		})
	}
}

func TestToolSuccessIsCountedAndTimed(t *testing.T) {
	tt := newToolTelemetry(t)
	callTool(t, tt.client, "get_post", map[string]any{"id": 1})

	inv := tt.metric(t, "mcp.tool.invocations").Data.(metricdata.Sum[int64])
	if len(inv.DataPoints) != 1 || pointAttr(inv.DataPoints[0].Attributes, "result") != "ok" {
		t.Errorf("invocations = %+v", inv.DataPoints)
	}
	if _, has := inv.DataPoints[0].Attributes.Value("error.type"); has {
		t.Error("a successful call must not carry error.type")
	}
	m := tt.metric(t, "mcp.tool.duration")
	hist, isHist := m.Data.(metricdata.Histogram[float64])
	if !isHist || m.Unit != "s" || len(hist.DataPoints) != 1 || hist.DataPoints[0].Count != 1 {
		t.Errorf("duration = %+v (unit %q)", m.Data, m.Unit)
	}
}
