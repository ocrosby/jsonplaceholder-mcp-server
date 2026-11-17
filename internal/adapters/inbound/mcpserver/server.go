// Package mcpserver is the inbound adapter: it exposes a ports.ResourceService
// as MCP tools, over stdio or stateless streamable HTTP.
//
// STDIO NOTE: never write to stdout in this package. Under stdio it is the
// JSON-RPC stream and any stray print corrupts it. Log via slog (stderr).
package mcpserver

import (
	"errors"
	"net/http"

	"github.com/jedi-knights/go-platform/httpserver"
	"github.com/mark3labs/mcp-go/server"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/ports"
)

// ServerName is reported to clients at initialize and is the resource.parent of every tool span.
const ServerName = "jsonplaceholder-mcp-server"

// New builds the MCP server with every tool registered against svc. Every tool
// call is traced and counted; see Option for the providers used.
func New(svc ports.ResourceService, version string, opts ...Option) (*server.MCPServer, error) {
	if svc == nil {
		return nil, errors.New("mcpserver: service is required")
	}
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	tel, err := newToolTelemetry(o)
	if err != nil {
		return nil, err
	}
	s := server.NewMCPServer(ServerName, version,
		server.WithInstructions(serverInstructions),
		server.WithToolCapabilities(false),
		server.WithToolHandlerMiddleware(tel.middleware),
	)
	register(s, svc)
	return s, nil
}

// NewHTTPHandler serves s over stateless streamable HTTP at endpoint, plus the
// /livez, /readyz and /health probes. Stateless means no session affinity, so
// any replica can answer any request.
func NewHTTPHandler(s *server.MCPServer, endpoint string) http.Handler {
	mux := http.NewServeMux()
	// go-platform's probes: the server holds no credentials and no state, so once
	// it is serving it is live and ready (ReadyHandler with no checks).
	mux.Handle("GET /livez", httpserver.HealthHandler())
	mux.Handle("GET /readyz", httpserver.ReadyHandler())
	mux.Handle("GET /health", httpserver.HealthHandler())
	mux.Handle(endpoint, server.NewStreamableHTTPServer(s,
		server.WithStateLess(true),
		server.WithEndpointPath(endpoint),
	))
	return mux
}
