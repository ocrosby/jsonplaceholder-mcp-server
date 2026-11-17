// Package jsonplaceholder is the outbound adapter: it implements
// ports.ResourceSource over the JSONPlaceholder REST API.
package jsonplaceholder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/domain"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/ports"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/requestlog"
)

// maxBody caps how much of a response is read. With the service's page-size
// cap no legitimate response approaches this; it bounds a misbehaving upstream.
const maxBody = 4 << 20

// Client talks to a JSONPlaceholder-compatible API.
type Client struct {
	baseURL string
	http    *http.Client
}

var _ ports.ResourceSource = (*Client)(nil)

// New builds a Client. baseURL is e.g. "https://jsonplaceholder.typicode.com"
// (no trailing slash); hc must not be nil and should carry a timeout.
func New(baseURL string, hc *http.Client) (*Client, error) {
	if baseURL == "" {
		return nil, errors.New("jsonplaceholder: base URL is required")
	}
	if hc == nil {
		return nil, errors.New("jsonplaceholder: http client is required")
	}
	return &Client{baseURL: baseURL, http: hc}, nil
}

// fetch GETs path (with query) and decodes the JSON body into T.
// what names the target for error messages, e.g. "post 7".
func fetch[T any](ctx context.Context, c *Client, path string, q url.Values, what string) (T, error) {
	var zero T
	target := c.baseURL + path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	requestlog.Record(ctx, "GET "+target)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return zero, domain.Upstream(0, "building request for %s: %v", what, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return zero, domain.Upstream(0, "requesting %s: %v", what, err)
	}
	defer func() { _ = resp.Body.Close() }() // read-only body; a close error has no consequence

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return zero, domain.NotFound("%s not found", what)
	case resp.StatusCode != http.StatusOK:
		return zero, domain.Upstream(resp.StatusCode, "%s: upstream returned HTTP %d", what, resp.StatusCode)
	}

	var out T
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&out); err != nil {
		// 502: the upstream answered 200 but not with usable JSON.
		return zero, domain.Upstream(http.StatusBadGateway, "%s: undecodable upstream response: %v", what, err)
	}
	return out, nil
}

func listQuery(o domain.ListOptions) url.Values {
	q := url.Values{}
	q.Set("_page", strconv.Itoa(o.Page))
	q.Set("_limit", strconv.Itoa(o.Limit))
	return q
}

func setID(q url.Values, key string, id *int) {
	if id != nil {
		q.Set(key, strconv.Itoa(*id))
	}
}

func one[T any](ctx context.Context, c *Client, collection, noun string, id int) (T, error) {
	return fetch[T](ctx, c, "/"+collection+"/"+strconv.Itoa(id), nil, fmt.Sprintf("%s %d", noun, id))
}

// ListPosts implements ports.ResourceSource.
func (c *Client) ListPosts(ctx context.Context, f domain.PostFilter, o domain.ListOptions) ([]domain.Post, error) {
	q := listQuery(o)
	setID(q, "userId", f.UserID)
	return fetch[[]domain.Post](ctx, c, "/posts", q, "posts")
}

// GetPost implements ports.ResourceSource.
func (c *Client) GetPost(ctx context.Context, id int) (domain.Post, error) {
	return one[domain.Post](ctx, c, "posts", "post", id)
}

// ListComments implements ports.ResourceSource.
func (c *Client) ListComments(ctx context.Context, f domain.CommentFilter, o domain.ListOptions) ([]domain.Comment, error) {
	q := listQuery(o)
	setID(q, "postId", f.PostID)
	return fetch[[]domain.Comment](ctx, c, "/comments", q, "comments")
}

// GetComment implements ports.ResourceSource.
func (c *Client) GetComment(ctx context.Context, id int) (domain.Comment, error) {
	return one[domain.Comment](ctx, c, "comments", "comment", id)
}

// ListAlbums implements ports.ResourceSource.
func (c *Client) ListAlbums(ctx context.Context, f domain.AlbumFilter, o domain.ListOptions) ([]domain.Album, error) {
	q := listQuery(o)
	setID(q, "userId", f.UserID)
	return fetch[[]domain.Album](ctx, c, "/albums", q, "albums")
}

// GetAlbum implements ports.ResourceSource.
func (c *Client) GetAlbum(ctx context.Context, id int) (domain.Album, error) {
	return one[domain.Album](ctx, c, "albums", "album", id)
}

// ListPhotos implements ports.ResourceSource.
func (c *Client) ListPhotos(ctx context.Context, f domain.PhotoFilter, o domain.ListOptions) ([]domain.Photo, error) {
	q := listQuery(o)
	setID(q, "albumId", f.AlbumID)
	return fetch[[]domain.Photo](ctx, c, "/photos", q, "photos")
}

// GetPhoto implements ports.ResourceSource.
func (c *Client) GetPhoto(ctx context.Context, id int) (domain.Photo, error) {
	return one[domain.Photo](ctx, c, "photos", "photo", id)
}

// ListTodos implements ports.ResourceSource.
func (c *Client) ListTodos(ctx context.Context, f domain.TodoFilter, o domain.ListOptions) ([]domain.Todo, error) {
	q := listQuery(o)
	setID(q, "userId", f.UserID)
	if f.Completed != nil {
		q.Set("completed", strconv.FormatBool(*f.Completed))
	}
	return fetch[[]domain.Todo](ctx, c, "/todos", q, "todos")
}

// GetTodo implements ports.ResourceSource.
func (c *Client) GetTodo(ctx context.Context, id int) (domain.Todo, error) {
	return one[domain.Todo](ctx, c, "todos", "todo", id)
}

// ListUsers implements ports.ResourceSource.
func (c *Client) ListUsers(ctx context.Context, o domain.ListOptions) ([]domain.User, error) {
	return fetch[[]domain.User](ctx, c, "/users", listQuery(o), "users")
}

// GetUser implements ports.ResourceSource.
func (c *Client) GetUser(ctx context.Context, id int) (domain.User, error) {
	return one[domain.User](ctx, c, "users", "user", id)
}
