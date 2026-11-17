package application_test

import (
	"context"
	"testing"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/application"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/domain"
	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/ports"
)

// fakeSource records what the service passes through; unset methods panic via the nil embed.
type fakeSource struct {
	ports.ResourceSource
	gotOpts   domain.ListOptions
	gotFilter domain.PostFilter
	calls     int
}

func (f *fakeSource) ListPosts(_ context.Context, fl domain.PostFilter, o domain.ListOptions) ([]domain.Post, error) {
	f.calls++
	f.gotOpts, f.gotFilter = o, fl
	return []domain.Post{{ID: 1}}, nil
}

func (f *fakeSource) GetPost(_ context.Context, id int) (domain.Post, error) {
	f.calls++
	return domain.Post{ID: id}, nil
}

func newSvc(t *testing.T) (*application.Service, *fakeSource) {
	t.Helper()
	src := &fakeSource{}
	svc, err := application.NewService(src)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, src
}

func kindOf(err error) domain.Kind {
	if de, ok := domain.As(err); ok {
		return de.Kind
	}
	return 0
}

func TestNewServiceRequiresSource(t *testing.T) {
	if _, err := application.NewService(nil); err == nil {
		t.Fatal("expected error for nil source")
	}
}

func TestListAppliesDefaultsAndPassesFilter(t *testing.T) {
	svc, src := newSvc(t)
	uid := 3
	if _, err := svc.ListPosts(context.Background(), domain.PostFilter{UserID: &uid}, domain.ListOptions{}); err != nil {
		t.Fatal(err)
	}
	if src.gotOpts != (domain.ListOptions{Page: 1, Limit: application.DefaultLimit}) {
		t.Errorf("opts = %+v, want page 1 limit %d", src.gotOpts, application.DefaultLimit)
	}
	if src.gotFilter.UserID == nil || *src.gotFilter.UserID != 3 {
		t.Errorf("filter not passed through: %+v", src.gotFilter)
	}
}

func TestListLimitBoundary(t *testing.T) {
	svc, src := newSvc(t)
	if _, err := svc.ListPosts(context.Background(), domain.PostFilter{}, domain.ListOptions{Limit: application.MaxLimit}); err != nil {
		t.Fatalf("limit == MaxLimit must be accepted: %v", err)
	}
	if src.gotOpts.Limit != application.MaxLimit {
		t.Errorf("limit = %d", src.gotOpts.Limit)
	}
	_, err := svc.ListPosts(context.Background(), domain.PostFilter{}, domain.ListOptions{Limit: application.MaxLimit + 1})
	if kindOf(err) != domain.KindInvalid {
		t.Errorf("limit above cap: err = %v, want KindInvalid", err)
	}
}

func TestInvalidInputNeverReachesSource(t *testing.T) {
	zero, neg := 0, -4
	cases := map[string]func(*application.Service) error{
		"get id 0":        func(s *application.Service) error { _, e := s.GetPost(context.Background(), 0); return e },
		"get id negative": func(s *application.Service) error { _, e := s.GetPost(context.Background(), -1); return e },
		"filter user 0": func(s *application.Service) error {
			_, e := s.ListPosts(context.Background(), domain.PostFilter{UserID: &zero}, domain.ListOptions{})
			return e
		},
		"negative page": func(s *application.Service) error {
			_, e := s.ListPosts(context.Background(), domain.PostFilter{}, domain.ListOptions{Page: neg})
			return e
		},
		"negative limit": func(s *application.Service) error {
			_, e := s.ListPosts(context.Background(), domain.PostFilter{}, domain.ListOptions{Limit: neg})
			return e
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			svc, src := newSvc(t)
			if kindOf(run(svc)) != domain.KindInvalid {
				t.Error("want KindInvalid")
			}
			if src.calls != 0 {
				t.Errorf("source called %d times", src.calls)
			}
		})
	}
}

// allSource passes everything through successfully so each Service method's happy path is observable.
type allSource struct{ ports.ResourceSource }

func (allSource) ListComments(context.Context, domain.CommentFilter, domain.ListOptions) ([]domain.Comment, error) {
	return []domain.Comment{{ID: 1}}, nil
}
func (allSource) GetComment(_ context.Context, id int) (domain.Comment, error) {
	return domain.Comment{ID: id}, nil
}
func (allSource) ListAlbums(context.Context, domain.AlbumFilter, domain.ListOptions) ([]domain.Album, error) {
	return []domain.Album{{ID: 1}}, nil
}
func (allSource) GetAlbum(_ context.Context, id int) (domain.Album, error) {
	return domain.Album{ID: id}, nil
}
func (allSource) ListPhotos(context.Context, domain.PhotoFilter, domain.ListOptions) ([]domain.Photo, error) {
	return []domain.Photo{{ID: 1}}, nil
}
func (allSource) GetPhoto(_ context.Context, id int) (domain.Photo, error) {
	return domain.Photo{ID: id}, nil
}
func (allSource) ListTodos(context.Context, domain.TodoFilter, domain.ListOptions) ([]domain.Todo, error) {
	return []domain.Todo{{ID: 1}}, nil
}
func (allSource) GetTodo(_ context.Context, id int) (domain.Todo, error) {
	return domain.Todo{ID: id}, nil
}
func (allSource) ListUsers(context.Context, domain.ListOptions) ([]domain.User, error) {
	return []domain.User{{ID: 1}}, nil
}
func (allSource) GetUser(_ context.Context, id int) (domain.User, error) {
	return domain.User{ID: id}, nil
}

// TestEveryResourceValidatesAndDelegates drives all twelve operations: a bad id or filter is
// rejected as KindInvalid, a good one reaches the source and returns its data.
func TestEveryResourceValidatesAndDelegates(t *testing.T) {
	svc, err := application.NewService(allSource{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, bad, good := context.Background(), 0, 2
	type op struct {
		name   string
		reject func() error
		accept func() error
	}
	ops := []op{
		{"comments", func() error {
			_, e := svc.ListComments(ctx, domain.CommentFilter{PostID: &bad}, domain.ListOptions{})
			return e
		},
			func() error {
				_, e := svc.ListComments(ctx, domain.CommentFilter{PostID: &good}, domain.ListOptions{})
				return e
			}},
		{"albums", func() error {
			_, e := svc.ListAlbums(ctx, domain.AlbumFilter{UserID: &bad}, domain.ListOptions{})
			return e
		},
			func() error {
				_, e := svc.ListAlbums(ctx, domain.AlbumFilter{UserID: &good}, domain.ListOptions{})
				return e
			}},
		{"photos", func() error {
			_, e := svc.ListPhotos(ctx, domain.PhotoFilter{AlbumID: &bad}, domain.ListOptions{})
			return e
		},
			func() error {
				_, e := svc.ListPhotos(ctx, domain.PhotoFilter{AlbumID: &good}, domain.ListOptions{})
				return e
			}},
		{"todos", func() error {
			_, e := svc.ListTodos(ctx, domain.TodoFilter{UserID: &bad}, domain.ListOptions{})
			return e
		},
			func() error {
				_, e := svc.ListTodos(ctx, domain.TodoFilter{UserID: &good}, domain.ListOptions{})
				return e
			}},
		{"users", func() error {
			_, e := svc.ListUsers(ctx, domain.ListOptions{Limit: application.MaxLimit + 1})
			return e
		},
			func() error { _, e := svc.ListUsers(ctx, domain.ListOptions{}); return e }},
		{"get comment", func() error { _, e := svc.GetComment(ctx, 0); return e }, func() error { _, e := svc.GetComment(ctx, 1); return e }},
		{"get album", func() error { _, e := svc.GetAlbum(ctx, 0); return e }, func() error { _, e := svc.GetAlbum(ctx, 1); return e }},
		{"get photo", func() error { _, e := svc.GetPhoto(ctx, 0); return e }, func() error { _, e := svc.GetPhoto(ctx, 1); return e }},
		{"get todo", func() error { _, e := svc.GetTodo(ctx, 0); return e }, func() error { _, e := svc.GetTodo(ctx, 1); return e }},
		{"get user", func() error { _, e := svc.GetUser(ctx, 0); return e }, func() error { _, e := svc.GetUser(ctx, 1); return e }},
	}
	for _, o := range ops {
		t.Run(o.name, func(t *testing.T) {
			if kindOf(o.reject()) != domain.KindInvalid {
				t.Error("invalid input must be KindInvalid")
			}
			if err := o.accept(); err != nil {
				t.Errorf("valid input rejected: %v", err)
			}
		})
	}
}
