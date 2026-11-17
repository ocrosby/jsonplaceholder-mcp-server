package mcpserver

import (
	"context"
	"strconv"
	"time"

	platformotel "github.com/jedi-knights/go-platform/otel"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const (
	instrumentationName = "github.com/ocrosby/jsonplaceholder-mcp-server/internal/adapters/inbound/mcpserver"

	// spanToolInvoke follows go-platform's short.snake_case span naming
	// ("tool.invoke" is its own example).
	spanToolInvoke = "tool.invoke"

	metricInvocations = "mcp.tool.invocations"
	metricDuration    = "mcp.tool.duration"

	resourceKindTool = "tool"
	actionInvoke     = "invoke"

	// errorTypeInvalid is error.type for a request rejected before any upstream call.
	errorTypeInvalid  = "invalid_request"
	errorTypeInternal = "internal"
	attrResult        = "result"
	attrErrorType     = "error.type"
)

// Option configures New.
type Option func(*options)

type options struct {
	tracerProvider trace.TracerProvider
	meterProvider  metric.MeterProvider
}

// WithTracerProvider sets the TracerProvider tool spans are created with. The
// default is the global provider, which is a no-op until the application
// installs one.
func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(o *options) { o.tracerProvider = tp }
}

// WithMeterProvider sets the MeterProvider tool metrics are created with. The
// default is the global provider.
func WithMeterProvider(mp metric.MeterProvider) Option {
	return func(o *options) { o.meterProvider = mp }
}

// toolTelemetry instruments every tool call at the adapter boundary, as the
// portfolio's observability strategy requires (never inside the use case).
// It records the tool's name and outcome, never its arguments.
type toolTelemetry struct {
	tracer      trace.Tracer
	invocations metric.Int64Counter
	duration    metric.Float64Histogram
}

func newToolTelemetry(o options) (*toolTelemetry, error) {
	tp, mp := o.tracerProvider, o.meterProvider
	if tp == nil {
		tp = otel.GetTracerProvider()
	}
	if mp == nil {
		mp = otel.GetMeterProvider()
	}
	meter := mp.Meter(instrumentationName)
	invocations, err := meter.Int64Counter(metricInvocations,
		metric.WithUnit("{call}"), metric.WithDescription("MCP tool calls, by tool and result."))
	if err != nil {
		return nil, err
	}
	duration, err := meter.Float64Histogram(metricDuration,
		metric.WithUnit("s"), metric.WithDescription("Duration of an MCP tool call."))
	if err != nil {
		return nil, err
	}
	return &toolTelemetry{tracer: tp.Tracer(instrumentationName), invocations: invocations, duration: duration}, nil
}

// toolAttrs is the portfolio vocabulary for one tool (go-platform otel/attrs.go,
// ADR-0019): resource_path is "<server>/tool/<tool>".
func toolAttrs(tool string) []attribute.KeyValue {
	attrs := platformotel.Resource("tool:"+tool, resourceKindTool, tool, ServerName, ServerName+"/"+resourceKindTool+"/"+tool)
	return append(attrs,
		attribute.String(platformotel.AttrToolName, tool),
		attribute.String(platformotel.AttrAction, actionInvoke))
}

// middleware wraps a tool handler in a span and records its metrics.
func (t *toolTelemetry) middleware(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tool := req.Params.Name
		attrs := toolAttrs(tool)
		ctx, span := t.tracer.Start(ctx, spanToolInvoke, trace.WithAttributes(attrs...))
		began := time.Now()

		res, err := next(ctx, req)

		metricAttrs := []attribute.KeyValue{attribute.String(platformotel.AttrToolName, tool), attribute.String(attrResult, "ok")}
		if errType := errorType(res, err); errType != "" {
			metricAttrs[1] = attribute.String(attrResult, "error")
			metricAttrs = append(metricAttrs, attribute.String(attrErrorType, errType))
			span.SetAttributes(attribute.String(attrErrorType, errType))
			span.SetStatus(codes.Error, errType)
		}
		span.End()
		t.invocations.Add(ctx, 1, metric.WithAttributes(metricAttrs...))
		t.duration.Record(ctx, time.Since(began).Seconds(), metric.WithAttributes(metricAttrs...))
		return res, err
	}
}

// errorType classifies a finished call: "" on success, otherwise the upstream
// HTTP status, "invalid_request" when no request was made, or "internal".
func errorType(res *mcp.CallToolResult, err error) string {
	if err != nil || res == nil {
		return errorTypeInternal
	}
	if !res.IsError {
		return ""
	}
	if status, ok := upstreamStatus(res); ok {
		return strconv.Itoa(status)
	}
	return errorTypeInvalid
}

// upstreamStatus reads status_code out of the structured result that
// buildResult attaches; it is absent when the call never reached the network.
func upstreamStatus(res *mcp.CallToolResult) (int, bool) {
	m, ok := res.StructuredContent.(map[string]any)
	if !ok {
		return 0, false
	}
	r, ok := m["result"].(map[string]any)
	if !ok {
		return 0, false
	}
	status, ok := r["status_code"].(*int)
	if !ok || status == nil {
		return 0, false
	}
	return *status, true
}
