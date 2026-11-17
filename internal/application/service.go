// Package application validates requests and delegates to the outbound port.
// It owns every input rule (positive ids, bounded page size) so adapters stay thin.
package application

import (
	"context"
	"errors"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/domain"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/ports"
)

// Page-size policy. The upstream /photos collection has 5000 rows, so every
// list is bounded: MaxLimit is the hard cap on one response.
const (
	DefaultLimit = 20
	MaxLimit     = 100
)

// Service implements ports.ResourceService over a ports.ResourceSource.
type Service struct{ src ports.ResourceSource }

var _ ports.ResourceService = (*Service)(nil)

// NewService builds a Service. src must not be nil.
func NewService(src ports.ResourceSource) (*Service, error) {
	if src == nil {
		return nil, errors.New("application: resource source is required")
	}
	return &Service{src: src}, nil
}

func normalize(o domain.ListOptions) (domain.ListOptions, error) {
	if o.Page < 0 {
		return o, domain.Invalid("page must be 1 or greater, got %d", o.Page)
	}
	if o.Limit < 0 || o.Limit > MaxLimit {
		return o, domain.Invalid("limit must be between 1 and %d, got %d", MaxLimit, o.Limit)
	}
	if o.Page == 0 {
		o.Page = 1
	}
	if o.Limit == 0 {
		o.Limit = DefaultLimit
	}
	return o, nil
}

func checkID(name string, id int) error {
	if id < 1 {
		return domain.Invalid("%s must be 1 or greater, got %d", name, id)
	}
	return nil
}

func checkOptID(name string, id *int) error {
	if id == nil {
		return nil
	}
	return checkID(name, *id)
}

// ListPosts lists posts, optionally for one user.
func (s *Service) ListPosts(ctx context.Context, f domain.PostFilter, o domain.ListOptions) ([]domain.Post, error) {
	if err := checkOptID("user_id", f.UserID); err != nil {
		return nil, err
	}
	o, err := normalize(o)
	if err != nil {
		return nil, err
	}
	return s.src.ListPosts(ctx, f, o)
}

// GetPost fetches one post.
func (s *Service) GetPost(ctx context.Context, id int) (domain.Post, error) {
	if err := checkID("id", id); err != nil {
		return domain.Post{}, err
	}
	return s.src.GetPost(ctx, id)
}

// ListComments lists comments, optionally for one post.
func (s *Service) ListComments(ctx context.Context, f domain.CommentFilter, o domain.ListOptions) ([]domain.Comment, error) {
	if err := checkOptID("post_id", f.PostID); err != nil {
		return nil, err
	}
	o, err := normalize(o)
	if err != nil {
		return nil, err
	}
	return s.src.ListComments(ctx, f, o)
}

// GetComment fetches one comment.
func (s *Service) GetComment(ctx context.Context, id int) (domain.Comment, error) {
	if err := checkID("id", id); err != nil {
		return domain.Comment{}, err
	}
	return s.src.GetComment(ctx, id)
}

// ListAlbums lists albums, optionally for one user.
func (s *Service) ListAlbums(ctx context.Context, f domain.AlbumFilter, o domain.ListOptions) ([]domain.Album, error) {
	if err := checkOptID("user_id", f.UserID); err != nil {
		return nil, err
	}
	o, err := normalize(o)
	if err != nil {
		return nil, err
	}
	return s.src.ListAlbums(ctx, f, o)
}

// GetAlbum fetches one album.
func (s *Service) GetAlbum(ctx context.Context, id int) (domain.Album, error) {
	if err := checkID("id", id); err != nil {
		return domain.Album{}, err
	}
	return s.src.GetAlbum(ctx, id)
}

// ListPhotos lists photos, optionally for one album.
func (s *Service) ListPhotos(ctx context.Context, f domain.PhotoFilter, o domain.ListOptions) ([]domain.Photo, error) {
	if err := checkOptID("album_id", f.AlbumID); err != nil {
		return nil, err
	}
	o, err := normalize(o)
	if err != nil {
		return nil, err
	}
	return s.src.ListPhotos(ctx, f, o)
}

// GetPhoto fetches one photo.
func (s *Service) GetPhoto(ctx context.Context, id int) (domain.Photo, error) {
	if err := checkID("id", id); err != nil {
		return domain.Photo{}, err
	}
	return s.src.GetPhoto(ctx, id)
}

// ListTodos lists todos, optionally for one user and/or by completion.
func (s *Service) ListTodos(ctx context.Context, f domain.TodoFilter, o domain.ListOptions) ([]domain.Todo, error) {
	if err := checkOptID("user_id", f.UserID); err != nil {
		return nil, err
	}
	o, err := normalize(o)
	if err != nil {
		return nil, err
	}
	return s.src.ListTodos(ctx, f, o)
}

// GetTodo fetches one todo.
func (s *Service) GetTodo(ctx context.Context, id int) (domain.Todo, error) {
	if err := checkID("id", id); err != nil {
		return domain.Todo{}, err
	}
	return s.src.GetTodo(ctx, id)
}

// ListUsers lists users.
func (s *Service) ListUsers(ctx context.Context, o domain.ListOptions) ([]domain.User, error) {
	o, err := normalize(o)
	if err != nil {
		return nil, err
	}
	return s.src.ListUsers(ctx, o)
}

// GetUser fetches one user.
func (s *Service) GetUser(ctx context.Context, id int) (domain.User, error) {
	if err := checkID("id", id); err != nil {
		return domain.User{}, err
	}
	return s.src.GetUser(ctx, id)
}
