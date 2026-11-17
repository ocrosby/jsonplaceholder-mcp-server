// Package jsonplaceholder is the outbound adapter: it implements
// ports.ResourceSource by delegating to the jsonplaceholder-sdk client and
// translating between the SDK's types and errors and the domain's.
package jsonplaceholder

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	sdk "github.com/ocrosby/jsonplaceholder-sdk"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/domain"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/ports"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/requestlog"
)

// Client adapts the SDK to ports.ResourceSource.
type Client struct{ sdk *sdk.Client }

var _ ports.ResourceSource = (*Client)(nil)

// New builds a Client. baseURL is e.g. "https://jsonplaceholder.typicode.com"
// (no trailing slash); hc must not be nil and should carry a timeout, which
// applies to each attempt the SDK makes.
func New(baseURL string, hc *http.Client) (*Client, error) {
	c, err := sdk.NewClient(
		sdk.WithBaseURL(baseURL),
		sdk.WithHTTPClient(hc),
		// Every attempt, retries included, lands in the tool result's "API Requests:" footer.
		sdk.WithRequestHook(func(ctx context.Context, method, url string) {
			requestlog.Record(ctx, method+" "+url)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("jsonplaceholder: %w", err)
	}
	return &Client{sdk: c}, nil
}

// mapErr translates an SDK error into the domain error the application speaks.
// what names the target for messages, e.g. "post 7".
func mapErr(err error, what string) error {
	var apiErr *sdk.APIError
	switch {
	case errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound:
		return domain.NotFound("%s not found", what)
	case errors.As(err, &apiErr):
		return domain.Upstream(apiErr.StatusCode, "%s: upstream returned HTTP %d", what, apiErr.StatusCode)
	case errors.Is(err, sdk.ErrInvalidArgument):
		return domain.Invalid("%s: %v", what, err)
	case errors.Is(err, sdk.ErrInvalidResponse):
		// 502: the upstream answered 200 but not with usable JSON.
		return domain.Upstream(http.StatusBadGateway, "%s: undecodable upstream response: %v", what, err)
	default:
		return domain.Upstream(0, "requesting %s: %v", what, err)
	}
}

// opts converts the domain's paging options.
func opts(o domain.ListOptions) sdk.ListOptions {
	return sdk.ListOptions{Page: o.Page, Limit: o.Limit}
}

// convert maps a slice element by element.
func convert[S, D any](in []S, f func(S) D) []D {
	out := make([]D, len(in))
	for i, v := range in {
		out[i] = f(v)
	}
	return out
}

// The SDK and domain entities have identical fields, so Go struct conversion
// applies; User nests types, which need explicit mapping.
func toPost(p sdk.Post) domain.Post          { return domain.Post(p) }
func toComment(c sdk.Comment) domain.Comment { return domain.Comment(c) }
func toAlbum(a sdk.Album) domain.Album       { return domain.Album(a) }
func toPhoto(p sdk.Photo) domain.Photo       { return domain.Photo(p) }
func toTodo(t sdk.Todo) domain.Todo          { return domain.Todo(t) }

func toUser(u sdk.User) domain.User {
	return domain.User{
		ID: u.ID, Name: u.Name, Username: u.Username, Email: u.Email, Phone: u.Phone, Website: u.Website,
		Address: domain.Address{
			Street: u.Address.Street, Suite: u.Address.Suite, City: u.Address.City, Zipcode: u.Address.Zipcode,
			Geo: domain.Geo{Lat: u.Address.Geo.Lat, Lng: u.Address.Geo.Lng},
		},
		Company: domain.Company{Name: u.Company.Name, CatchPhrase: u.Company.CatchPhrase, BS: u.Company.BS},
	}
}

// ListPosts implements ports.ResourceSource.
func (c *Client) ListPosts(ctx context.Context, f domain.PostFilter, o domain.ListOptions) ([]domain.Post, error) {
	page, err := c.sdk.Posts.List(ctx, sdk.PostListParams{UserID: f.UserID, ListOptions: opts(o)})
	if err != nil {
		return nil, mapErr(err, "posts")
	}
	return convert(page.Items, toPost), nil
}

// GetPost implements ports.ResourceSource.
func (c *Client) GetPost(ctx context.Context, id int) (domain.Post, error) {
	p, err := c.sdk.Posts.Get(ctx, id)
	if err != nil {
		return domain.Post{}, mapErr(err, fmt.Sprintf("post %d", id))
	}
	return toPost(*p), nil
}

// ListComments implements ports.ResourceSource.
func (c *Client) ListComments(ctx context.Context, f domain.CommentFilter, o domain.ListOptions) ([]domain.Comment, error) {
	page, err := c.sdk.Comments.List(ctx, sdk.CommentListParams{PostID: f.PostID, ListOptions: opts(o)})
	if err != nil {
		return nil, mapErr(err, "comments")
	}
	return convert(page.Items, toComment), nil
}

// GetComment implements ports.ResourceSource.
func (c *Client) GetComment(ctx context.Context, id int) (domain.Comment, error) {
	v, err := c.sdk.Comments.Get(ctx, id)
	if err != nil {
		return domain.Comment{}, mapErr(err, fmt.Sprintf("comment %d", id))
	}
	return toComment(*v), nil
}

// ListAlbums implements ports.ResourceSource.
func (c *Client) ListAlbums(ctx context.Context, f domain.AlbumFilter, o domain.ListOptions) ([]domain.Album, error) {
	page, err := c.sdk.Albums.List(ctx, sdk.AlbumListParams{UserID: f.UserID, ListOptions: opts(o)})
	if err != nil {
		return nil, mapErr(err, "albums")
	}
	return convert(page.Items, toAlbum), nil
}

// GetAlbum implements ports.ResourceSource.
func (c *Client) GetAlbum(ctx context.Context, id int) (domain.Album, error) {
	v, err := c.sdk.Albums.Get(ctx, id)
	if err != nil {
		return domain.Album{}, mapErr(err, fmt.Sprintf("album %d", id))
	}
	return toAlbum(*v), nil
}

// ListPhotos implements ports.ResourceSource.
func (c *Client) ListPhotos(ctx context.Context, f domain.PhotoFilter, o domain.ListOptions) ([]domain.Photo, error) {
	page, err := c.sdk.Photos.List(ctx, sdk.PhotoListParams{AlbumID: f.AlbumID, ListOptions: opts(o)})
	if err != nil {
		return nil, mapErr(err, "photos")
	}
	return convert(page.Items, toPhoto), nil
}

// GetPhoto implements ports.ResourceSource.
func (c *Client) GetPhoto(ctx context.Context, id int) (domain.Photo, error) {
	v, err := c.sdk.Photos.Get(ctx, id)
	if err != nil {
		return domain.Photo{}, mapErr(err, fmt.Sprintf("photo %d", id))
	}
	return toPhoto(*v), nil
}

// ListTodos implements ports.ResourceSource.
func (c *Client) ListTodos(ctx context.Context, f domain.TodoFilter, o domain.ListOptions) ([]domain.Todo, error) {
	page, err := c.sdk.Todos.List(ctx, sdk.TodoListParams{UserID: f.UserID, Completed: f.Completed, ListOptions: opts(o)})
	if err != nil {
		return nil, mapErr(err, "todos")
	}
	return convert(page.Items, toTodo), nil
}

// GetTodo implements ports.ResourceSource.
func (c *Client) GetTodo(ctx context.Context, id int) (domain.Todo, error) {
	v, err := c.sdk.Todos.Get(ctx, id)
	if err != nil {
		return domain.Todo{}, mapErr(err, fmt.Sprintf("todo %d", id))
	}
	return toTodo(*v), nil
}

// ListUsers implements ports.ResourceSource.
func (c *Client) ListUsers(ctx context.Context, o domain.ListOptions) ([]domain.User, error) {
	page, err := c.sdk.Users.List(ctx, opts(o))
	if err != nil {
		return nil, mapErr(err, "users")
	}
	return convert(page.Items, toUser), nil
}

// GetUser implements ports.ResourceSource.
func (c *Client) GetUser(ctx context.Context, id int) (domain.User, error) {
	v, err := c.sdk.Users.Get(ctx, id)
	if err != nil {
		return domain.User{}, mapErr(err, fmt.Sprintf("user %d", id))
	}
	return toUser(*v), nil
}
