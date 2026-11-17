package mcpserver

import (
	"context"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/domain"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/requestlog"
)

const httpOK = 200

// buildResult wraps prose, the raw payload and the recorded upstream requests
// into a CallToolResult, in the same shape as sun-mcp's build_tool_result:
//
//	content           prose, plus an "API Requests:" footer when any were made
//	structuredContent {"result": {"prose", "payload", "status_code"}, "api_requests": [...]}
//	isError           true for every status other than 200 (including a null status)
//
// status is nil when the call never produced a response (validation or
// connection failure).
func buildResult(prose string, payload any, status *int, rec *requestlog.Recorder) *mcp.CallToolResult {
	entries := rec.Entries()
	text := prose
	if len(entries) > 0 {
		text += "\n\n---\nAPI Requests:"
		for _, e := range entries {
			text += "\n  " + e
		}
	}
	res := mcp.NewToolResultStructured(map[string]any{
		"result": map[string]any{
			"prose":       prose,
			"payload":     payload,
			"status_code": status,
		},
		"api_requests": entries,
	}, text)
	res.IsError = status == nil || *status != httpOK
	return res
}

// errorResult maps a domain error to an isError result with sun-mcp's wording.
func errorResult(ctx context.Context, err error, rec *requestlog.Recorder) *mcp.CallToolResult {
	de, ok := domain.As(err)
	if !ok {
		slog.ErrorContext(ctx, "unexpected error", "err", err)
		msg := "Internal error: " + err.Error()
		return buildResult(msg, msg, nil, rec)
	}
	var msg string
	switch de.Kind {
	case domain.KindInvalid:
		slog.WarnContext(ctx, "invalid request", "err", de)
		msg = "Invalid request: " + de.Message
	case domain.KindNotFound:
		slog.WarnContext(ctx, "not found", "err", de)
		msg = "Not found: " + de.Message
	default:
		slog.ErrorContext(ctx, "upstream error", "err", de)
		msg = "Upstream error: " + de.Message
	}
	var status *int
	if de.Status != 0 {
		s := de.Status
		status = &s
	}
	return buildResult(msg, msg, status, rec)
}

func okResult(prose string, payload any, rec *requestlog.Recorder) *mcp.CallToolResult {
	s := httpOK
	return buildResult(prose, payload, &s, rec)
}

// compactMiddleware removes the raw payload from structuredContent, keeping
// prose, status_code and api_requests. buildResult makes a fresh map per call,
// so deleting in place cannot leak into another result.
func compactMiddleware(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		res, err := next(ctx, req)
		if res == nil { // a handler error carries no result to compact
			return res, err
		}
		if m, ok := res.StructuredContent.(map[string]any); ok {
			if r, ok := m["result"].(map[string]any); ok {
				delete(r, "payload")
			}
		}
		return res, err
	}
}
