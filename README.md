# jsonplaceholder-mcp-server

A [Model Context Protocol](https://modelcontextprotocol.io) server, written in Go, that gives an AI assistant read-only access to [JSONPlaceholder](https://jsonplaceholder.typicode.com): users, posts, comments, albums, photos and todos.

<p align="center">
  <a href="#what-it-does">What it does</a> · <a href="#install">Install</a> · <a href="#connect-a-client">Connect a client</a> · <a href="#example-prompts">Example prompts</a> · <a href="#tools">Tools</a> · <a href="#configuration">Configuration</a> · <a href="#architecture">Architecture</a> · <a href="#development">Development</a>
</p>

## What it does

Once connected, your assistant can look up JSONPlaceholder's fake REST data by calling tools instead of guessing or you pasting JSON. It is useful as a safe, no-credentials MCP server for learning MCP, demoing tool use, or testing a client. The data is static test data, not real people or content.

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
# Probes:        /livez  /readyz  /health

claude mcp add --transport http jsonplaceholder http://localhost:8000/mcp
```

## Example prompts

Ask in plain language; the assistant chooses the tools, filters and paging. The right column shows what it typically calls.

| Ask | Tools used |
|---|---|
| "Who are the users? Give me names and emails." | `list_users` |
| "Tell me about user 4: where do they live and what company do they work for?" | `get_user` |
| "Show me the first three posts by user 2." | `list_posts` (`user_id`, `limit`) |
| "Read post 12 and summarize it." | `get_post` |
| "What are the comments on post 7, and who left them?" | `list_comments` (`post_id`) |
| "Find the comment with id 250." | `get_comment` |
| "What albums does user 3 own?" | `list_albums` (`user_id`) |
| "Show the first five photos in album 10, with their URLs." | `list_photos` (`album_id`, `limit`) |
| "Which of user 2's todos are still open?" | `list_todos` (`user_id`, `completed`) |
| "Is todo 55 done?" | `get_todo` |

Questions that span resources work too, because the assistant chains calls:

- "Which user wrote post 30, and what do their other posts say?" (`get_post`, `get_user`, `list_posts`)
- "For user 5, how many todos are done versus open?" (`list_todos` twice, once per `completed` value)
- "Pick a random album, name its owner, and list three photo titles." (`list_albums`, `get_user`, `list_photos`)

Asking for something that does not exist, such as post 9999, returns a clear "not found" result rather than an error.

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
