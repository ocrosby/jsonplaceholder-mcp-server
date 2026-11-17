// Package mcpserver is the inbound adapter: it exposes a ports.ResourceService
// as MCP tools, over stdio or stateless streamable HTTP.
//
// STDIO NOTE: never write to stdout in this package. Under stdio it is the
// JSON-RPC stream and any stray print corrupts it. Log via slog (stderr).
package mcpserver

import (
	"errors"
	"net/http"

	"github.com/mark3labs/mcp-go/server"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/ports"
)

// ServerName is reported to clients at initialize.
const ServerName = "jsonplaceholder-mcp-server"

// New builds the MCP server with every tool registered against svc.
func New(svc ports.ResourceService, version string) (*server.MCPServer, error) {
	if svc == nil {
		return nil, errors.New("mcpserver: service is required")
	}
	s := server.NewMCPServer(ServerName, version,
		server.WithInstructions(serverInstructions),
		server.WithToolCapabilities(false),
	)
	register(s, svc)
	return s, nil
}

// NewHTTPHandler serves s over stateless streamable HTTP at endpoint, plus the
// /livez, /readyz and /health probes. Stateless means no session affinity, so
// any replica can answer any request.
func NewHTTPHandler(s *server.MCPServer, endpoint string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", handleLivez)
	mux.HandleFunc("GET /readyz", handleReadyz)
	mux.HandleFunc("GET /health", handleHealth)
	mux.Handle(endpoint, server.NewStreamableHTTPServer(s,
		server.WithStateLess(true),
		server.WithEndpointPath(endpoint),
	))
	return mux
}
