package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/domain"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/ports"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/requestlog"
)

// toolSpec describes one tool for discovery: clients and agents can group and
// filter tools by resource and operation without parsing names or prose.
type toolSpec struct{ name, resource, operation string }

// toolSpecs is the single source of truth for the tool set, in registration order.
var toolSpecs = []toolSpec{
	{"list_users", "users", "list"}, {"get_user", "users", "get"},
	{"list_posts", "posts", "list"}, {"get_post", "posts", "get"},
	{"list_comments", "comments", "list"}, {"get_comment", "comments", "get"},
	{"list_albums", "albums", "list"}, {"get_album", "albums", "get"},
	{"list_photos", "photos", "list"}, {"get_photo", "photos", "get"},
	{"list_todos", "todos", "list"}, {"get_todo", "todos", "get"},
}

// ToolNames lists every registered tool, in registration order.
var ToolNames = func() []string {
	names := make([]string, len(toolSpecs))
	for i, sp := range toolSpecs {
		names[i] = sp.name
	}
	return names
}()

// MetaKey is the reverse-DNS-prefixed _meta key carrying discovery metadata:
// {"resource": "posts", "operation": "list"|"get", "paged": bool}.
// Clients that do not know it ignore it; descriptions and the server
// instructions remain the primary discovery surface.
const MetaKey = "io.github.ocrosby/jsonplaceholder"

// withDiscoveryMeta attaches the _meta block for the named tool. An unknown
// name is a programming error (the tool is not in toolSpecs), so it panics at
// startup rather than shipping an undiscoverable tool.
func withDiscoveryMeta(name string) mcp.ToolOption {
	for _, sp := range toolSpecs {
		if sp.name == name {
			return func(t *mcp.Tool) {
				t.Meta = &mcp.Meta{AdditionalFields: map[string]any{
					MetaKey: map[string]any{
						"resource":  sp.resource,
						"operation": sp.operation,
						"paged":     sp.operation == "list",
					},
				}}
			}
		}
	}
	panic("invariant: tool " + name + " is not in toolSpecs")
}

// readTool builds a tool carrying sun-mcp's read-only annotations:
// readOnly, non-destructive, idempotent, openWorld (it calls an external API).
func readTool(name, title, description string, opts ...mcp.ToolOption) mcp.Tool {
	base := []mcp.ToolOption{
		mcp.WithDescription(description),
		mcp.WithTitleAnnotation(title),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		withDiscoveryMeta(name),
	}
	return mcp.NewTool(name, append(base, opts...)...)
}

func idArg(noun string) mcp.ToolOption {
	return mcp.WithNumber("id", mcp.Required(), mcp.Min(1), mcp.Description("The "+noun+" id, a whole number of 1 or greater."))
}

func filterArg(name, description string) mcp.ToolOption {
	return mcp.WithNumber(name, mcp.Min(1), mcp.Description(description))
}

func pagingArgs() []mcp.ToolOption {
	return []mcp.ToolOption{
		mcp.WithNumber("page", mcp.Min(1), mcp.Description("Page number, 1-based. Default 1.")),
		mcp.WithNumber("limit", mcp.Min(1), mcp.Max(100), mcp.Description("Items per page. Default 20, maximum 100.")),
	}
}

// call adapts a typed operation into a ToolHandlerFunc: it scopes a request
// recorder to the call, runs fn, and renders the outcome as a CallToolResult.
// Tool failures are results with isError set, never JSON-RPC errors.
func call[T any](fn func(ctx context.Context, req mcp.CallToolRequest) (T, error), format func(T) string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, rec := requestlog.WithRecorder(ctx)
		v, err := fn(ctx, req)
		if err != nil {
			return errorResult(err, rec), nil
		}
		return okResult(format(v), v, rec), nil
	}
}

// getByID builds the handler for a "get one" tool.
func getByID[T any](get func(ctx context.Context, id int) (T, error), format func(T) string) server.ToolHandlerFunc {
	return call(func(ctx context.Context, req mcp.CallToolRequest) (T, error) {
		var zero T
		id, err := reqInt(req, "id")
		if err != nil {
			return zero, err
		}
		return get(ctx, id)
	}, format)
}

// listTool builds a paged list tool: extra filter args first, then page and limit.
func listTool(name, title, desc string, extra ...mcp.ToolOption) mcp.Tool {
	return readTool(name, title, desc, append(extra, pagingArgs()...)...)
}

// register wires every tool to svc.
func register(s *server.MCPServer, svc ports.ResourceService) {
	registerUsers(s, svc)
	registerPosts(s, svc)
	registerComments(s, svc)
	registerAlbums(s, svc)
	registerPhotos(s, svc)
	registerTodos(s, svc)
}

// registerUsers registers list_users and get_user.
func registerUsers(s *server.MCPServer, svc ports.ResourceService) {
	s.AddTool(listTool("list_users", "List users",
		"List users: the 10 people in JSONPlaceholder, with name, username, email, address, phone, website and company. Use to find a user id, look someone up by name or email, or see who works where."),
		call(func(ctx context.Context, req mcp.CallToolRequest) ([]domain.User, error) {
			o, err := listOptions(req)
			if err != nil {
				return nil, err
			}
			return svc.ListUsers(ctx, o)
		}, func(v []domain.User) string { return fmtList("users", v, fmtUser) }))
	s.AddTool(readTool("get_user", "Get a user", "Get one user by id: name, username, email, phone, website, company and a full address with coordinates. Use when you already have a user id and need their profile or contact details.", idArg("user")),
		getByID(svc.GetUser, fmtUser))
}

// registerPosts registers list_posts and get_post.
func registerPosts(s *server.MCPServer, svc ports.ResourceService) {
	s.AddTool(listTool("list_posts", "List posts",
		"List or find posts (100 in total, 10 per user): id, title and body. Use for 'what did user N write' by passing user_id, or to browse posts page by page.",
		filterArg("user_id", "Only posts written by this user id.")),
		call(func(ctx context.Context, req mcp.CallToolRequest) ([]domain.Post, error) {
			uid, err := optInt(req, "user_id")
			if err != nil {
				return nil, err
			}
			o, err := listOptions(req)
			if err != nil {
				return nil, err
			}
			return svc.ListPosts(ctx, domain.PostFilter{UserID: uid}, o)
		}, func(v []domain.Post) string { return fmtList("posts", v, fmtPost) }))
	s.AddTool(readTool("get_post", "Get a post", "Get one post by id: title, body and the user who wrote it. Use to read or summarize a specific post.", idArg("post")),
		getByID(svc.GetPost, fmtPost))
}

// registerComments registers list_comments and get_comment.
func registerComments(s *server.MCPServer, svc ports.ResourceService) {
	s.AddTool(listTool("list_comments", "List comments",
		"List comments (500 in total, 5 per post): commenter name, email and body. Use for 'what are the comments on post N' by passing post_id.",
		filterArg("post_id", "Only comments on this post id.")),
		call(func(ctx context.Context, req mcp.CallToolRequest) ([]domain.Comment, error) {
			pid, err := optInt(req, "post_id")
			if err != nil {
				return nil, err
			}
			o, err := listOptions(req)
			if err != nil {
				return nil, err
			}
			return svc.ListComments(ctx, domain.CommentFilter{PostID: pid}, o)
		}, func(v []domain.Comment) string { return fmtList("comments", v, fmtComment) }))
	s.AddTool(readTool("get_comment", "Get a comment", "Get one comment by id: commenter name and email, body and the post it belongs to.", idArg("comment")),
		getByID(svc.GetComment, fmtComment))
}

// registerAlbums registers list_albums and get_album.
func registerAlbums(s *server.MCPServer, svc ports.ResourceService) {
	s.AddTool(listTool("list_albums", "List albums",
		"List photo albums (100 in total, 10 per user): id and title. Use for 'what albums does user N have' by passing user_id.",
		filterArg("user_id", "Only albums owned by this user id.")),
		call(func(ctx context.Context, req mcp.CallToolRequest) ([]domain.Album, error) {
			uid, err := optInt(req, "user_id")
			if err != nil {
				return nil, err
			}
			o, err := listOptions(req)
			if err != nil {
				return nil, err
			}
			return svc.ListAlbums(ctx, domain.AlbumFilter{UserID: uid}, o)
		}, func(v []domain.Album) string { return fmtList("albums", v, fmtAlbum) }))
	s.AddTool(readTool("get_album", "Get an album", "Get one album by id: its title and the user who owns it. Use to find an album's owner before or after list_photos.", idArg("album")),
		getByID(svc.GetAlbum, fmtAlbum))
}

// registerPhotos registers list_photos and get_photo.
func registerPhotos(s *server.MCPServer, svc ports.ResourceService) {
	s.AddTool(listTool("list_photos", "List photos",
		"List photos (5000 in total, 50 per album): title, full-size url and thumbnail url. Always pass album_id to list one album's photos; an unfiltered list is huge.",
		filterArg("album_id", "Only photos in this album id.")),
		call(func(ctx context.Context, req mcp.CallToolRequest) ([]domain.Photo, error) {
			aid, err := optInt(req, "album_id")
			if err != nil {
				return nil, err
			}
			o, err := listOptions(req)
			if err != nil {
				return nil, err
			}
			return svc.ListPhotos(ctx, domain.PhotoFilter{AlbumID: aid}, o)
		}, func(v []domain.Photo) string { return fmtList("photos", v, fmtPhoto) }))
	s.AddTool(readTool("get_photo", "Get a photo", "Get one photo by id: title, full-size url, thumbnail url and the album it is in. Use to show or describe a specific photo.", idArg("photo")),
		getByID(svc.GetPhoto, fmtPhoto))
}

// registerTodos registers list_todos and get_todo.
func registerTodos(s *server.MCPServer, svc ports.ResourceService) {
	s.AddTool(listTool("list_todos", "List todos",
		"List todos, also called to-do items or tasks (200 in total, 20 per user): title and done or open status. Use for 'what is user N working on' (user_id) and 'what is finished or still open' (completed true or false).",
		filterArg("user_id", "Only todos belonging to this user id."),
		mcp.WithBoolean("completed", mcp.Description("true for finished todos only, false for open ones only. Omit for both."))),
		call(func(ctx context.Context, req mcp.CallToolRequest) ([]domain.Todo, error) {
			uid, err := optInt(req, "user_id")
			if err != nil {
				return nil, err
			}
			done, err := optBool(req, "completed")
			if err != nil {
				return nil, err
			}
			o, err := listOptions(req)
			if err != nil {
				return nil, err
			}
			return svc.ListTodos(ctx, domain.TodoFilter{UserID: uid, Completed: done}, o)
		}, func(v []domain.Todo) string { return fmtList("todos", v, fmtTodo) }))
	s.AddTool(readTool("get_todo", "Get a todo", "Get one todo (to-do item or task) by id: title, whether it is completed and the user it belongs to. Use to check whether a specific todo is done.", idArg("todo")),
		getByID(svc.GetTodo, fmtTodo))
}
