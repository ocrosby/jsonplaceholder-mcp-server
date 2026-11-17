# jsonplaceholder-mcp-server

A [Model Context Protocol](https://modelcontextprotocol.io) server, written in Go, that gives an MCP client read-only access to [JSONPlaceholder](https://jsonplaceholder.typicode.com): users, posts, comments, albums, photos and todos.

<p align="center">
  <a href="#install">Install</a> · <a href="#usage">Usage</a> · <a href="#tools">Tools</a> · <a href="#configuration">Configuration</a> · <a href="#architecture">Architecture</a> · <a href="#development">Development</a>
</p>

## Install

```bash
go install github.com/ocrosby/jsonplaceholder-mcp-server/cmd/jsonplaceholder-mcp-server@latest
```

Or build from a checkout with `make build` (binary at `bin/jsonplaceholder-mcp-server`).

## Usage

**stdio (default)**: the client spawns the server. For Claude Code:

```bash
claude mcp add jsonplaceholder -- jsonplaceholder-mcp-server
```

**streamable-http**: stateless, for networked use.

```bash
MCP_TRANSPORT=streamable-http PORT=8000 jsonplaceholder-mcp-server
# MCP endpoint:  http://localhost:8000/mcp
# Probes:        /livez  /readyz  /health
claude mcp add --transport http jsonplaceholder http://localhost:8000/mcp
```

## Tools

All tools are read-only. JSONPlaceholder fakes writes and persists nothing, so there are no create, update or delete tools.

| Resource | List | Get one | Filters on list |
|---|---|---|---|
| Users | `list_users` | `get_user` | |
| Posts | `list_posts` | `get_post` | `user_id` |
| Comments | `list_comments` | `get_comment` | `post_id` |
| Albums | `list_albums` | `get_album` | `user_id` |
| Photos | `list_photos` | `get_photo` | `album_id` |
| Todos | `list_todos` | `get_todo` | `user_id`, `completed` |

Every list tool takes `page` (default 1) and `limit` (default 20, maximum 100). The upstream nested routes (`/posts/1/comments`) are covered by the filters, not separate tools.

Every result follows sun-mcp's shape: `content` is readable prose with an "API Requests:" footer, and `structuredContent` is `{"result": {"prose", "payload", "status_code"}, "api_requests": [...]}`. Failures (invalid input, not found, upstream error) are results with `isError: true`, not protocol errors.

## Configuration

All variables are optional. See `.env.example`.

| Variable | Default | Purpose |
|---|---|---|
| `MCP_TRANSPORT` | `stdio` | `stdio` or `streamable-http` |
| `HOST` / `PORT` | `0.0.0.0` / `8000` | Bind address (streamable-http only) |
| `LOG_LEVEL` | `INFO` | `DEBUG`, `INFO`, `WARN`, `ERROR`; logs go to stderr |
| `API_HOST` | `https://jsonplaceholder.typicode.com` | Upstream base URL |
| `HTTP_TIMEOUT_SECONDS` | `10` | Per upstream request, 1-120 |

## Architecture

Hexagonal: the domain and application layers know nothing about MCP or HTTP.

```
cmd/jsonplaceholder-mcp-server   composition root
internal/domain                  entities, filters, errors
internal/ports                   ResourceSource (out), ResourceService (in)
internal/application             validation, page-size cap
internal/adapters/inbound/mcpserver        MCP tools, result shape, probes
internal/adapters/outbound/jsonplaceholder HTTP client
```

## Development

```bash
make test    # go test -race ./...
make lint    # golangci-lint
make vuln    # govulncheck
docker build -t jsonplaceholder-mcp-server .
```
