package jsonplaceholder_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/adapters/outbound/jsonplaceholder"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/domain"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/requestlog"
)

func newClient(t *testing.T, h http.HandlerFunc) *jsonplaceholder.Client {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	c, err := jsonplaceholder.New(ts.URL, ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestNewValidates(t *testing.T) {
	if _, err := jsonplaceholder.New("", http.DefaultClient); err == nil {
		t.Error("empty base URL must error")
	}
	if _, err := jsonplaceholder.New("http://x", nil); err == nil {
		t.Error("nil client must error")
	}
}

func TestListTodosBuildsQueryAndDecodes(t *testing.T) {
	var got url.Values
	var path string
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		path, got = r.URL.Path, r.URL.Query()
		_, _ = w.Write([]byte(`[{"userId":2,"id":41,"title":"t","completed":true}]`))
	})
	uid, done := 2, true
	todos, err := c.ListTodos(context.Background(), domain.TodoFilter{UserID: &uid, Completed: &done}, domain.ListOptions{Page: 3, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/todos" {
		t.Errorf("path = %q", path)
	}
	for k, want := range map[string]string{"userId": "2", "completed": "true", "_page": "3", "_limit": "5"} {
		if got.Get(k) != want {
			t.Errorf("query %s = %q, want %q", k, got.Get(k), want)
		}
	}
	if len(todos) != 1 || todos[0].ID != 41 || !todos[0].Completed || todos[0].UserID != 2 {
		t.Errorf("decoded %+v", todos)
	}
}

func TestOmittedFiltersAreNotSent(t *testing.T) {
	var got url.Values
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) { got = r.URL.Query(); _, _ = w.Write([]byte(`[]`)) })
	if _, err := c.ListPosts(context.Background(), domain.PostFilter{}, domain.ListOptions{Page: 1, Limit: 20}); err != nil {
		t.Fatal(err)
	}
	if got.Has("userId") {
		t.Errorf("userId sent without a filter: %v", got)
	}
}

func TestGetDecodesNestedUser(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":1,"name":"Leanne","address":{"city":"Gwenborough","geo":{"lat":"-37.3","lng":"81.1"}},"company":{"name":"Romaguera"}}`))
	})
	u, err := c.GetUser(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if u.Address.City != "Gwenborough" || u.Address.Geo.Lng != "81.1" || u.Company.Name != "Romaguera" {
		t.Errorf("decoded %+v", u)
	}
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		wantKind   domain.Kind
		wantStatus int
	}{
		{"404", 404, `{}`, domain.KindNotFound, 404},
		{"500", 500, `boom`, domain.KindUpstream, 500},
		{"429", 429, ``, domain.KindUpstream, 429},
		{"200 garbage", 200, `<html>`, domain.KindUpstream, 502},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			_, err := c.GetPost(context.Background(), 7)
			de, ok := domain.As(err)
			if !ok || de.Kind != tc.wantKind || de.Status != tc.wantStatus {
				t.Errorf("err = %#v, want kind %d status %d", de, tc.wantKind, tc.wantStatus)
			}
		})
	}
}

func TestNotFoundNamesTheResource(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) })
	_, err := c.GetAlbum(context.Background(), 999)
	if err == nil || err.Error() != "album 999 not found" {
		t.Errorf("err = %v", err)
	}
}

func TestConnectionFailureHasNoStatus(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	c, _ := jsonplaceholder.New(ts.URL, ts.Client())
	ts.Close()
	_, err := c.GetPost(context.Background(), 1)
	de, ok := domain.As(err)
	if !ok || de.Kind != domain.KindUpstream || de.Status != 0 {
		t.Errorf("err = %#v, want upstream with status 0", de)
	}
}

func TestRecordsRequests(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"id":1}`)) })
	ctx, rec := requestlog.WithRecorder(context.Background())
	if _, err := c.GetComment(ctx, 1); err != nil {
		t.Fatal(err)
	}
	e := rec.Entries()
	if len(e) != 1 || e[0][:4] != "GET " || e[0][len(e[0])-len("/comments/1"):] != "/comments/1" {
		t.Errorf("entries = %v", e)
	}
}

func TestTransientFailureIsRetriedAndEveryAttemptIsRecorded(t *testing.T) {
	attempts := 0
	c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"id":3}`))
	})
	ctx, rec := requestlog.WithRecorder(context.Background())
	p, err := c.GetPost(ctx, 3)
	if err != nil || p.ID != 3 {
		t.Fatalf("expected the retry to succeed: post=%+v err=%v", p, err)
	}
	if got := rec.Entries(); len(got) != 2 {
		t.Errorf("recorded %d requests, want 2 (the failed attempt and the retry): %v", len(got), got)
	}
}

// TestEveryOperationMapsSDKTypesToDomainTypes drives all twelve operations
// against canned JSON and checks each field survives the SDK-to-domain
// translation, including the nested user.
func TestEveryOperationMapsSDKTypesToDomainTypes(t *testing.T) {
	fixtures := map[string]string{
		"/users":      `[{"id":1,"name":"Leanne","username":"Bret","email":"e@x","phone":"p","website":"w","address":{"street":"s","suite":"u","city":"c","zipcode":"z","geo":{"lat":"1.5","lng":"2.5"}},"company":{"name":"Co","catchPhrase":"cp","bs":"b"}}]`,
		"/users/1":    `{"id":1,"name":"Leanne","username":"Bret","email":"e@x","phone":"p","website":"w","address":{"street":"s","suite":"u","city":"c","zipcode":"z","geo":{"lat":"1.5","lng":"2.5"}},"company":{"name":"Co","catchPhrase":"cp","bs":"b"}}`,
		"/posts":      `[{"userId":2,"id":3,"title":"t","body":"b"}]`,
		"/posts/3":    `{"userId":2,"id":3,"title":"t","body":"b"}`,
		"/comments":   `[{"postId":3,"id":4,"name":"n","email":"e","body":"b"}]`,
		"/comments/4": `{"postId":3,"id":4,"name":"n","email":"e","body":"b"}`,
		"/albums":     `[{"userId":2,"id":5,"title":"t"}]`,
		"/albums/5":   `{"userId":2,"id":5,"title":"t"}`,
		"/photos":     `[{"albumId":5,"id":6,"title":"t","url":"u","thumbnailUrl":"th"}]`,
		"/photos/6":   `{"albumId":5,"id":6,"title":"t","url":"u","thumbnailUrl":"th"}`,
		"/todos":      `[{"userId":2,"id":7,"title":"t","completed":true}]`,
		"/todos/7":    `{"userId":2,"id":7,"title":"t","completed":true}`,
	}
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, ok := fixtures[r.URL.Path]
		if !ok {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(body))
	})
	ctx, o := context.Background(), domain.ListOptions{Page: 1, Limit: 5}

	users, err := c.ListUsers(ctx, o)
	u := users[0]
	if err != nil || u.ID != 1 || u.Username != "Bret" || u.Address.City != "c" || u.Address.Geo.Lng != "2.5" || u.Company.BS != "b" || u.Company.CatchPhrase != "cp" {
		t.Errorf("ListUsers: %+v (%v)", users, err)
	}
	if one, err := c.GetUser(ctx, 1); err != nil || one != u {
		t.Errorf("GetUser: %+v (%v), want %+v", one, err, u)
	}
	if v, err := c.ListPosts(ctx, domain.PostFilter{}, o); err != nil || v[0] != (domain.Post{UserID: 2, ID: 3, Title: "t", Body: "b"}) {
		t.Errorf("ListPosts: %+v (%v)", v, err)
	}
	if v, err := c.GetPost(ctx, 3); err != nil || v.UserID != 2 || v.Body != "b" {
		t.Errorf("GetPost: %+v (%v)", v, err)
	}
	if v, err := c.ListComments(ctx, domain.CommentFilter{}, o); err != nil || v[0] != (domain.Comment{PostID: 3, ID: 4, Name: "n", Email: "e", Body: "b"}) {
		t.Errorf("ListComments: %+v (%v)", v, err)
	}
	if v, err := c.GetComment(ctx, 4); err != nil || v.PostID != 3 || v.Email != "e" {
		t.Errorf("GetComment: %+v (%v)", v, err)
	}
	if v, err := c.ListAlbums(ctx, domain.AlbumFilter{}, o); err != nil || v[0] != (domain.Album{UserID: 2, ID: 5, Title: "t"}) {
		t.Errorf("ListAlbums: %+v (%v)", v, err)
	}
	if v, err := c.GetAlbum(ctx, 5); err != nil || v.UserID != 2 {
		t.Errorf("GetAlbum: %+v (%v)", v, err)
	}
	if v, err := c.ListPhotos(ctx, domain.PhotoFilter{}, o); err != nil || v[0] != (domain.Photo{AlbumID: 5, ID: 6, Title: "t", URL: "u", ThumbnailURL: "th"}) {
		t.Errorf("ListPhotos: %+v (%v)", v, err)
	}
	if v, err := c.GetPhoto(ctx, 6); err != nil || v.ThumbnailURL != "th" {
		t.Errorf("GetPhoto: %+v (%v)", v, err)
	}
	if v, err := c.ListTodos(ctx, domain.TodoFilter{}, o); err != nil || v[0] != (domain.Todo{UserID: 2, ID: 7, Title: "t", Completed: true}) {
		t.Errorf("ListTodos: %+v (%v)", v, err)
	}
	if v, err := c.GetTodo(ctx, 7); err != nil || !v.Completed {
		t.Errorf("GetTodo: %+v (%v)", v, err)
	}
}
