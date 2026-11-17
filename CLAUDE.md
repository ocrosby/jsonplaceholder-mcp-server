# jsonplaceholder-mcp-server

Go MCP server exposing read-only tools over jsonplaceholder.typicode.com. Mirrors sun-mcp's protocol conventions; hexagonal layout.

## Layout (dependencies point inward)

- `internal/domain`: entities, filters, `*domain.Error` (Invalid / NotFound / Upstream). Stdlib only.
- `internal/ports`: `ResourceSource` (outbound) and `ResourceService` (inbound).
- `internal/application`: input validation and the page-size cap; the only place rules live.
- `internal/adapters/outbound/jsonplaceholder`: adapter over [jsonplaceholder-sdk](https://github.com/ocrosby/jsonplaceholder-sdk). Maps SDK types and errors to domain ones; the SDK's request hook records every attempt (retries included) via `internal/requestlog`. Retries, backoff and HTTP live in the SDK, not here.
- `internal/adapters/inbound/mcpserver`: tools, result shaping, health probes, HTTP handler.
- `internal/bootstrap`: the go-platform wiring (telemetry mode, `otel.New`, metrics listener, `otelhttp` + `httpmw` chain, `httpserver` lifecycle). Only this package and `mcpserver` import go-platform's lifecycle packages.
- `cmd/jsonplaceholder-mcp-server`: composition root only.

## Protocol conventions (match sun-mcp; do not drift)

- Tool failures are `CallToolResult` with `isError: true`, never JSON-RPC errors. `isError` is true for any status other than 200, including a null status.
- `structuredContent` is `{"result": {"prose","payload","status_code"}, "api_requests": [...]}`; `content` is the prose plus an "API Requests:" footer.
- Every tool carries readOnly / non-destructive / idempotent / openWorld annotations.
- streamable-http is stateless, serves `/mcp`, plus `/livez` `/readyz` `/health`.
- Never write to stdout (it is the stdio JSON-RPC stream); log with `slog` to stderr. go-platform's `otel` defaults send logs **and** (with no OTLP endpoint) trace spans to stdout: `bootstrap` forces stderr and metrics-only mode never samples. `cmd/.../main_test.go` runs the real binary to prove stdout stays JSON-RPC in every mode; keep it green.
- Telemetry is inert unless `OTEL_EXPORTER_OTLP_ENDPOINT` or `METRICS_PORT` is set. Tool spans/metrics (`tool.invoke`, `mcp.tool.*`) live in `mcpserver/telemetry.go` and record the tool name and outcome, never arguments.
- `OTEL_EXPORTER_OTLP_ENDPOINT` is a *base* URL: `config.Load` appends `/v1/traces` for http protocols, because the exporter (since otlptrace v1.45) uses the URL as-is.

## Adding a tool

Add the method to `ports.ResourceSource`, then the client, the service (validation), a `register*` function in `tools.go`, an entry in `toolSpecs` (it feeds `ToolNames` and each tool's `_meta`), and the instructions text. Descriptions must lead with the words people ask with (a test enforces the resource noun). Prefer a filter on an existing list tool over a new tool (nested routes are filters).

## Commands

`make test` · `make lint` · `make vuln` · `make run-http`

The `go.mod` pins `x/net`, `grpc` and `otlptrace*` above what go-platform requires, because go-platform's pins carry reachable vulnerabilities; do not downgrade them (`make vuln` fails if you do).
