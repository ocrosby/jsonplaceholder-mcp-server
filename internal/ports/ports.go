// Package ports declares the two boundaries of the hexagon.
//
//   - ResourceSource is the outbound port: how the application reads
//     JSONPlaceholder data. adapters/outbound/jsonplaceholder implements it.
//   - ResourceService is the inbound port: what drivers (the MCP adapter)
//     may ask of the application. application.Service implements it.
package ports

import (
	"context"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/domain"
)

// ResourceSource is the outbound port. Implementations return *domain.Error
// values (KindNotFound, KindUpstream) and never validate caller input.
type ResourceSource interface {
	ListPosts(ctx context.Context, f domain.PostFilter, o domain.ListOptions) ([]domain.Post, error)
	GetPost(ctx context.Context, id int) (domain.Post, error)

	ListComments(ctx context.Context, f domain.CommentFilter, o domain.ListOptions) ([]domain.Comment, error)
	GetComment(ctx context.Context, id int) (domain.Comment, error)

	ListAlbums(ctx context.Context, f domain.AlbumFilter, o domain.ListOptions) ([]domain.Album, error)
	GetAlbum(ctx context.Context, id int) (domain.Album, error)

	ListPhotos(ctx context.Context, f domain.PhotoFilter, o domain.ListOptions) ([]domain.Photo, error)
	GetPhoto(ctx context.Context, id int) (domain.Photo, error)

	ListTodos(ctx context.Context, f domain.TodoFilter, o domain.ListOptions) ([]domain.Todo, error)
	GetTodo(ctx context.Context, id int) (domain.Todo, error)

	ListUsers(ctx context.Context, o domain.ListOptions) ([]domain.User, error)
	GetUser(ctx context.Context, id int) (domain.User, error)
}

// ResourceService is the inbound port. It has the same operations as
// ResourceSource, but implementations validate and bound every input first
// and return KindInvalid errors for bad requests.
type ResourceService interface {
	ResourceSource
}
