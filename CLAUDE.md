# jsonplaceholder-mcp-server

Go MCP server exposing read-only tools over jsonplaceholder.typicode.com. Mirrors sun-mcp's protocol conventions; hexagonal layout.

## Layout (dependencies point inward)

- `internal/domain`: entities, filters, `*domain.Error` (Invalid / NotFound / Upstream). Stdlib only.
- `internal/ports`: `ResourceSource` (outbound) and `ResourceService` (inbound).
- `internal/application`: input validation and the page-size cap; the only place rules live.
- `internal/adapters/outbound/jsonplaceholder`: HTTP client. Records each request via `internal/requestlog`.
- `internal/adapters/inbound/mcpserver`: tools, result shaping, health probes, HTTP handler.
- `cmd/jsonplaceholder-mcp-server`: composition root only.

## Protocol conventions (match sun-mcp; do not drift)

- Tool failures are `CallToolResult` with `isError: true`, never JSON-RPC errors. `isError` is true for any status other than 200, including a null status.
- `structuredContent` is `{"result": {"prose","payload","status_code"}, "api_requests": [...]}`; `content` is the prose plus an "API Requests:" footer.
- Every tool carries readOnly / non-destructive / idempotent / openWorld annotations.
- streamable-http is stateless, serves `/mcp`, plus `/livez` `/readyz` `/health`.
- Never write to stdout (it is the stdio JSON-RPC stream); log with `slog` to stderr.

## Adding a tool

Add the method to `ports.ResourceSource`, then the client, the service (validation), a `register*` function in `tools.go`, the name in `ToolNames`, and the instructions text. Prefer a filter on an existing list tool over a new tool (nested routes are filters).

## Commands

`make test` · `make lint` · `make vuln` · `make run-http`
