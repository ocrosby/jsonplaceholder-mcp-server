package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/domain"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/ports"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/requestlog"
)

// ToolNames lists every registered tool, in registration order.
var ToolNames = []string{
	"list_users", "get_user",
	"list_posts", "get_post",
	"list_comments", "get_comment",
	"list_albums", "get_album",
	"list_photos", "get_photo",
	"list_todos", "get_todo",
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
		"List the 10 JSONPlaceholder users (name, username, email, address, phone, website, company). Use this to find a user id."),
		call(func(ctx context.Context, req mcp.CallToolRequest) ([]domain.User, error) {
			o, err := listOptions(req)
			if err != nil {
				return nil, err
			}
			return svc.ListUsers(ctx, o)
		}, func(v []domain.User) string { return fmtList("users", v, fmtUser) }))
	s.AddTool(readTool("get_user", "Get a user", "Get one user by id: name, username, email, address with coordinates, phone, website and company.", idArg("user")),
		getByID(svc.GetUser, fmtUser))
}

// registerPosts registers list_posts and get_post.
func registerPosts(s *server.MCPServer, svc ports.ResourceService) {
	s.AddTool(listTool("list_posts", "List posts",
		"List posts (100 in total, 10 per user). Pass user_id to get only that user's posts.",
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
	s.AddTool(readTool("get_post", "Get a post", "Get one post by id: title, body and the user who wrote it.", idArg("post")),
		getByID(svc.GetPost, fmtPost))
}

// registerComments registers list_comments and get_comment.
func registerComments(s *server.MCPServer, svc ports.ResourceService) {
	s.AddTool(listTool("list_comments", "List comments",
		"List comments (500 in total, 5 per post). Pass post_id to get the comments on one post.",
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
	s.AddTool(readTool("get_comment", "Get a comment", "Get one comment by id: author name and email, body and the post it belongs to.", idArg("comment")),
		getByID(svc.GetComment, fmtComment))
}

// registerAlbums registers list_albums and get_album.
func registerAlbums(s *server.MCPServer, svc ports.ResourceService) {
	s.AddTool(listTool("list_albums", "List albums",
		"List photo albums (100 in total, 10 per user). Pass user_id to get one user's albums.",
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
	s.AddTool(readTool("get_album", "Get an album", "Get one album by id: title and the user who owns it.", idArg("album")),
		getByID(svc.GetAlbum, fmtAlbum))
}

// registerPhotos registers list_photos and get_photo.
func registerPhotos(s *server.MCPServer, svc ports.ResourceService) {
	s.AddTool(listTool("list_photos", "List photos",
		"List photos (5000 in total, 50 per album). Always pass album_id to get one album's photos.",
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
	s.AddTool(readTool("get_photo", "Get a photo", "Get one photo by id: title, full-size url, thumbnail url and the album it is in.", idArg("photo")),
		getByID(svc.GetPhoto, fmtPhoto))
}

// registerTodos registers list_todos and get_todo.
func registerTodos(s *server.MCPServer, svc ports.ResourceService) {
	s.AddTool(listTool("list_todos", "List todos",
		"List to-do items (200 in total, 20 per user). Filter by user_id and/or completed.",
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
	s.AddTool(readTool("get_todo", "Get a todo", "Get one to-do item by id: title, whether it is completed and the user it belongs to.", idArg("todo")),
		getByID(svc.GetTodo, fmtTodo))
}
