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
