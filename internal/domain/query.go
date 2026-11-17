package domain

// ListOptions selects one page of a collection. Zero values mean "use the
// default"; the application layer normalizes and bounds them.
type ListOptions struct {
	Page  int // 1-based
	Limit int // items per page
}

// Filters narrow a collection. A nil field means "do not filter on it".
// They also replace the upstream nested routes (e.g. /posts/1/comments is
// CommentFilter{PostID: 1}), so each resource needs only one list operation.

// PostFilter narrows posts.
type PostFilter struct{ UserID *int }

// CommentFilter narrows comments.
type CommentFilter struct{ PostID *int }

// AlbumFilter narrows albums.
type AlbumFilter struct{ UserID *int }

// PhotoFilter narrows photos.
type PhotoFilter struct{ AlbumID *int }

// TodoFilter narrows todos.
type TodoFilter struct {
	UserID    *int
	Completed *bool
}
