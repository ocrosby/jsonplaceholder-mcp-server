# jsonplaceholder-mcp-server

A [Model Context Protocol](https://modelcontextprotocol.io) server, written in Go, that gives an AI assistant read-only access to [JSONPlaceholder](https://jsonplaceholder.typicode.com): users, posts, comments, albums, photos and todos.

<p align="center">
  <a href="#what-it-does">What it does</a> · <a href="#install">Install</a> · <a href="#connect-a-client">Connect a client</a> · <a href="#skipping-approval-prompts">Approvals</a> · <a href="#example-prompts">Example prompts</a> · <a href="#tools">Tools</a> · <a href="#reference">Reference</a> · <a href="#observability">Observability</a> · <a href="#configuration">Configuration</a> · <a href="#architecture">Architecture</a> · <a href="#development">Development</a>
</p>

## What it does

Once connected, your assistant can look up JSONPlaceholder's fake REST data by calling tools instead of guessing or you pasting JSON. It is useful as a safe, no-credentials MCP server for learning MCP, demoing tool use, or testing a client. The data is static test data, not real people or content. See the [JSONPlaceholder guide](https://jsonplaceholder.typicode.com/guide/) and the [Reference](#reference) links to explore it.

See [Example prompts](#example-prompts) for things to ask.

The assistant picks the right tool, filters and pages for you. Nothing is ever written: JSONPlaceholder fakes writes, so the server only reads.

## Install

Requires Go 1.26 or later.

```bash
go install github.com/ocrosby/jsonplaceholder-mcp-server/cmd/jsonplaceholder-mcp-server@latest
```

This puts the binary in `$(go env GOPATH)/bin` (usually `~/go/bin`). Make sure that directory is on your `PATH`, or use the full path in the commands below. You can also build from a checkout with `make build` (binary at `bin/jsonplaceholder-mcp-server`).

## Connect a client

### Claude Code

```bash
claude mcp add --scope user jsonplaceholder -- jsonplaceholder-mcp-server
```

`--scope user` makes the server available in every project. Use `--scope project` to share it with a team through `.mcp.json`, or omit the flag for just the current project. If the binary is not on your `PATH`, pass its full path instead of the bare name.

Verify the connection, then start a new session so the tools load:

```bash
claude mcp list        # jsonplaceholder: ... ✔ Connected
```

Inside Claude Code, `/mcp` shows the server and its 12 tools.

### Claude Desktop and other stdio clients

Add this to the client's MCP config (for Claude Desktop, `claude_desktop_config.json`), using the full path to the binary:

```json
{
  "mcpServers": {
    "jsonplaceholder": {
      "command": "/Users/you/go/bin/jsonplaceholder-mcp-server"
    }
  }
}
```

### Over HTTP

For networked or shared use, run the server with the stateless streamable-http transport and point a client at it:

```bash
MCP_TRANSPORT=streamable-http PORT=8000 jsonplaceholder-mcp-server
# MCP endpoint:  http://localhost:8000/mcp
# Probes:        /livez  /readyz  /health (go-platform httpserver)

claude mcp add --transport http jsonplaceholder http://localhost:8000/mcp
```

## Skipping approval prompts

Clients ask before each tool call by default, so a first session can mean twelve separate prompts. A server cannot approve its own calls; that decision belongs to the client. This server is safe to allow in one step: every tool is read-only (`readOnlyHint`), non-destructive and idempotent, and the only thing it contacts is `jsonplaceholder.typicode.com`, which needs no credentials.

**Claude Code.** Add one rule to your user settings (`~/.claude/settings.json`), using the name you registered the server under (`jsonplaceholder` in the commands above):

```json
{
  "permissions": {
    "allow": ["mcp__jsonplaceholder"]
  }
}
```

- **The rule matches the server name, not the tool.** `mcp__jsonplaceholder` allows all 12 tools. If you registered it under another name, use `mcp__<that-name>`.
- **To allow only some tools**, list them individually: `mcp__jsonplaceholder__list_posts`, `mcp__jsonplaceholder__get_post`, and so on.
- **A read-only hint does not auto-approve.** Clients may use hints to label a tool, but Claude Code still prompts until a rule allows it.
- **Put the rule in user settings, not a shared file,** so each person opts in to what their own assistant may run.

**Claude Desktop** has no settings file for this: choose "Always allow" in the permission prompt for each tool.

## Example prompts

Ask in plain language; the assistant chooses the tools, filters and paging. The right column shows what it typically calls. Every tool appears at least once below.

### Users

| Ask | Tools used |
|---|---|
| "Who are the users? Give me names and emails." | `list_users` |
| "Tell me about user 4: where do they live and what company do they work for?" | `get_user` |
| "What are user 7's coordinates and website?" | `get_user` |
| "Which user works at Romaguera-Crona?" | `list_users` |

### Posts

| Ask | Tools used |
|---|---|
| "Show me the first three posts by user 2." | `list_posts` (`user_id`, `limit`) |
| "Read post 12 and summarize it." | `get_post` |
| "List every post title written by user 9." | `list_posts` (`user_id`) |

### Comments

| Ask | Tools used |
|---|---|
| "What are the comments on post 7, and who left them?" | `list_comments` (`post_id`) |
| "Find the comment with id 250 and tell me which post it is on." | `get_comment` |
| "Show two comments on post 1 with their email addresses." | `list_comments` (`post_id`, `limit`) |

### Albums and photos

| Ask | Tools used |
|---|---|
| "What albums does user 3 own?" | `list_albums` (`user_id`) |
| "Tell me about album 25: its title and its owner's id." | `get_album` |
| "Show the first five photos in album 10, with their URLs." | `list_photos` (`album_id`, `limit`) |
| "Describe photo 4000: title, full-size URL and thumbnail." | `get_photo` |
| "List the thumbnail URLs for the first ten photos in album 1." | `list_photos` (`album_id`, `limit`) |

### Todos

| Ask | Tools used |
|---|---|
| "Which of user 2's todos are still open?" | `list_todos` (`user_id`, `completed: false`) |
| "What has user 2 already finished?" | `list_todos` (`user_id`, `completed: true`) |
| "Is todo 55 done?" | `get_todo` |
| "Show me three open todos from anyone." | `list_todos` (`completed: false`, `limit`) |

### Paging

| Ask | Tools used |
|---|---|
| "Show the second page of posts, five per page." | `list_posts` (`page: 2`, `limit: 5`) |
| "Give me users 6 through 10." | `list_users` (`page: 2`, `limit: 5`) |
| "Show the comments on post 1 two at a time, starting with the second pair." | `list_comments` (`post_id`, `page: 2`, `limit: 2`) |

Lists default to 20 items per page and never return more than 100.

### Across resources

The assistant chains calls when a question spans resources:

- "Which user wrote post 30, and what do their other posts say?" (`get_post`, `get_user`, `list_posts`)
- "For user 5, how many todos are done versus open?" (`list_todos` twice, once per `completed` value)
- "Pick an album, name its owner, and list three photo titles." (`list_albums`, `get_user`, `list_photos`)
- "Who commented on post 3, and do any of them share an email domain with the post's author?" (`get_post`, `get_user`, `list_comments`)
- "Give me a one-paragraph profile of user 1: their company, how many posts and albums they have, and how many todos are open." (`get_user`, `list_posts`, `list_albums`, `list_todos`)

### Things that should fail cleanly

Failures come back as results the assistant can read and explain, not as protocol errors.

| Ask | What happens |
|---|---|
| "Show me post 9999." | "Not found" (HTTP 404) |
| "Get user 0." or "Get post -3." | "Invalid request: id must be 1 or greater", no upstream call is made |
| "List 500 posts at once." | "Invalid request: limit must be between 1 and 100" |
| "Create a new post titled Hello." | Not available: the server is read-only, so the assistant should say so |

## Tools

All tools are read-only. JSONPlaceholder fakes writes and persists nothing, so there are no create, update or delete tools.

Click a resource name to open its live data on JSONPlaceholder.

| Resource | Records | List | Get one | Filters on list |
|---|---|---|---|---|
| [Users](https://jsonplaceholder.typicode.com/users) | 10 | `list_users` | `get_user` | |
| [Posts](https://jsonplaceholder.typicode.com/posts) | 100 | `list_posts` | `get_post` | `user_id` |
| [Comments](https://jsonplaceholder.typicode.com/comments) | 500 | `list_comments` | `get_comment` | `post_id` |
| [Albums](https://jsonplaceholder.typicode.com/albums) | 100 | `list_albums` | `get_album` | `user_id` |
| [Photos](https://jsonplaceholder.typicode.com/photos) | 5000 | `list_photos` | `get_photo` | `album_id` |
| [Todos](https://jsonplaceholder.typicode.com/todos) | 200 | `list_todos` | `get_todo` | `user_id`, `completed` |

Every list tool takes `page` (default 1) and `limit` (default 20, maximum 100). The upstream nested routes (`/posts/1/comments`) are covered by the filters, not separate tools.

Every result follows sun-mcp's shape: `content` is readable prose with an "API Requests:" footer, and `structuredContent` is `{"result": {"prose", "payload", "status_code"}, "api_requests": [...]}`. Failures (invalid input, not found, upstream error) are results with `isError: true`, not protocol errors.

## Reference

Use these to see what the data looks like, before or while you ask for it.

**JSONPlaceholder**

- [jsonplaceholder.typicode.com](https://jsonplaceholder.typicode.com/): the home page, with the resource list and live examples.
- [Guide](https://jsonplaceholder.typicode.com/guide/): how the upstream API works, including routes, filtering, nested resources and paging.
- Browse the data: [users](https://jsonplaceholder.typicode.com/users), [posts](https://jsonplaceholder.typicode.com/posts), [comments](https://jsonplaceholder.typicode.com/comments), [albums](https://jsonplaceholder.typicode.com/albums), [photos](https://jsonplaceholder.typicode.com/photos), [todos](https://jsonplaceholder.typicode.com/todos).
- [typicode/jsonplaceholder](https://github.com/typicode/jsonplaceholder) is its source, and [typicode/json-server](https://github.com/typicode/json-server) is the server it is built on (it defines the `_page` and `_limit` paging this server uses).

**Model Context Protocol**

- [modelcontextprotocol.io](https://modelcontextprotocol.io): the protocol documentation.
- [Tools specification](https://modelcontextprotocol.io/specification/latest/server/tools): how tools, annotations and results are defined.

**How a tool call maps to an upstream request**

Every result ends with an "API Requests:" footer listing the exact URLs it called, so you can paste one into a browser and compare.

| Tool call | Upstream request |
|---|---|
| `list_posts` with `user_id=1`, `page=2`, `limit=5` | [`GET /posts?userId=1&_page=2&_limit=5`](https://jsonplaceholder.typicode.com/posts?userId=1&_page=2&_limit=5) |
| `get_post` with `id=1` | [`GET /posts/1`](https://jsonplaceholder.typicode.com/posts/1) |
| `list_comments` with `post_id=1` | [`GET /comments?postId=1`](https://jsonplaceholder.typicode.com/comments?postId=1) (the same data as [`/posts/1/comments`](https://jsonplaceholder.typicode.com/posts/1/comments)) |
| `list_todos` with `user_id=2`, `completed=false` | [`GET /todos?userId=2&completed=false`](https://jsonplaceholder.typicode.com/todos?userId=2&completed=false) |
| `list_photos` with `album_id=1` | [`GET /photos?albumId=1`](https://jsonplaceholder.typicode.com/photos?albumId=1) |

## Observability

The server emits OpenTelemetry traces and Prometheus metrics, built on [go-platform](https://github.com/jedi-knights/go-platform) and [jsonplaceholder-sdk](https://github.com/ocrosby/jsonplaceholder-sdk). It follows sun-mcp's rule that **unset means inert**: with no `OTEL_EXPORTER_OTLP_ENDPOINT` and no `METRICS_PORT`, nothing is installed and nothing is exported.

| Mode | Set | What happens |
|---|---|---|
| Inert | neither | No telemetry providers. Spans and metrics are no-ops. |
| Metrics only | `METRICS_PORT` | Prometheus metrics at `http://HOST:METRICS_PORT/metrics`. No spans are recorded. |
| Full | `OTEL_EXPORTER_OTLP_ENDPOINT` (plus optionally `METRICS_PORT`) | Traces are exported over OTLP as well. |

Try it locally with the standard OpenTelemetry environment variables:

```bash
METRICS_PORT=9464 \
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf \
MCP_TRANSPORT=streamable-http jsonplaceholder-mcp-server
curl -s localhost:9464/metrics | grep -E 'mcp_tool|jsonplaceholder_client'
```

What you get:

| Signal | Name | Notes |
|---|---|---|
| Span | `tool.invoke` | One per tool call, with `tool.name`, `action=invoke` and the portfolio's `resource.*` attributes (`resource.path` is `jsonplaceholder-mcp-server/tool/<tool>`). Arguments are never recorded. |
| Span | `jsonplaceholder.<resource>.<operation>` and `HTTP GET` | From the SDK: one per upstream call, with a child span per attempt and `retry` events. |
| Span | the service name | One server span per HTTP request, continuing the caller's `traceparent`. The probes are not traced. |
| Metric | `mcp.tool.invocations` | Counter by `tool.name`, `result` (`ok` or `error`) and `error.type`. |
| Metric | `mcp.tool.duration` | Histogram, seconds. |
| Metric | `jsonplaceholder.client.operation_duration`, `jsonplaceholder.client.retries` | From the SDK. |
| Metric | `http.server.*`, `http.client.*`, `go_*` | Standard HTTP and Go runtime metrics. |
| Log | stderr, JSON | Every record carries `trace_id` and `span_id` when it is emitted inside a span, and each HTTP response carries an `X-Trace-ID` header. |

Things to know:

- **stdout stays clean under stdio.** It is the JSON-RPC stream, so logs go to stderr, and metrics-only mode records no spans because go-platform's default trace exporter writes to stdout. An end-to-end test runs the real binary in all three modes and checks every stdout line is JSON-RPC.
- **Metrics-only ignores incoming trace context.** go-platform's sampler is parent-based, so honoring a caller's sampled `traceparent` would force spans to be recorded, and its default exporter would print them to stdout. Metrics-only mode extracts no trace context, so it never records. Configure an OTLP endpoint to have traces continue across services.
- **Probes.** `/livez`, `/readyz` and `/health` come from go-platform's `httpserver` and return `{"status":"ok"}`. The old aggregate `checks` field is gone.
- **Graceful shutdown.** On SIGINT or SIGTERM the HTTP server drains in-flight requests, then buffered spans are flushed (5 second limit).

## Configuration

All variables are optional. See `.env.example`.

| Variable | Default | Purpose |
|---|---|---|
| `MCP_TRANSPORT` | `stdio` | `stdio` or `streamable-http` |
| `HOST` / `PORT` | `0.0.0.0` / `8000` | Bind address (streamable-http only) |
| `LOG_LEVEL` | `INFO` | `DEBUG`, `INFO`, `WARN`, `ERROR`; logs go to stderr |
| `API_HOST` | `https://jsonplaceholder.typicode.com` | Upstream base URL |
| `COMPACT_RESULTS` | `false` | `true` drops the raw `payload` from `structuredContent`, roughly halving each result; prose, `status_code` and `api_requests` stay |
| `HTTP_TIMEOUT_SECONDS` | `10` | Per upstream attempt, 1-120. The SDK retries transient failures twice, so a call can take up to three attempts |
| `OTEL_SERVICE_NAME` | `jsonplaceholder-mcp-server` | OpenTelemetry `service.name` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | *(unset)* | Where to export traces (an http(s) URL). Unset means no trace export |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `grpc` | `grpc` or `http/protobuf`. With `http/protobuf`, `/v1/traces` is appended to the endpoint, per the OpenTelemetry spec |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | *(unset)* | A complete trace endpoint URL, used as-is. Wins over `OTEL_EXPORTER_OTLP_ENDPOINT` |
| `METRICS_PORT` | *(unset)* | Serve Prometheus metrics at `/metrics` on this port, on a separate listener. Unset means no listener. Must differ from `PORT` over HTTP. The fleet convention is `9464` |

## Architecture

Hexagonal: the domain and application layers know nothing about MCP or HTTP. The outbound adapter is a thin translation layer over [jsonplaceholder-sdk](https://github.com/ocrosby/jsonplaceholder-sdk), which owns the HTTP, retries and paging.

```
cmd/jsonplaceholder-mcp-server   composition root
internal/domain                  entities, filters, errors
internal/ports                   ResourceSource (out), ResourceService (in)
internal/application             validation, page-size cap
internal/adapters/inbound/mcpserver        MCP tools, result shape, probes
internal/adapters/outbound/jsonplaceholder adapter over jsonplaceholder-sdk
```

## Development

```bash
make test    # go test -race ./...
make lint    # golangci-lint
make vuln    # govulncheck
docker build -t jsonplaceholder-mcp-server .
```
