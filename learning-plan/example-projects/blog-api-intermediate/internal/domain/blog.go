package domain

import (
	"context"
	"errors"
	"time"
)

// ErrPostNotFound is returned when a post does not exist.
var ErrPostNotFound = errors.New("post not found")

// ValidationError signals invalid input; the handler maps it to HTTP 400.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.Field != "" {
		return e.Field + ": " + e.Message
	}
	return e.Message
}

// Post is the core entity. It owns many Comments and is labelled with Tags.
type Post struct {
	ID           int64
	Title        string
	Slug         string
	Body         string
	Published    bool
	Tags         []string
	Comments     []Comment
	CommentCount int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Comment belongs to a Post.
type Comment struct {
	ID        int64
	PostID    int64
	Author    string
	Body      string
	CreatedAt time.Time
}

// PostFilter describes listing options: search, tag/published filters, paging.
type PostFilter struct {
	Search    string
	Tag       string
	Published *bool // nil = any
	Page      int
	PageSize  int
}

// Page is a generic paginated result set.
type Page[T any] struct {
	Items      []T
	Page       int
	PageSize   int
	Total      int64
	TotalPages int
}

// PostRepository is the storage contract for posts and their comments.
type PostRepository interface {
	CreatePost(ctx context.Context, p *Post) error
	GetPostByID(ctx context.Context, id int64) (*Post, error)
	ListPosts(ctx context.Context, f PostFilter) (items []Post, total int64, err error)
	UpdatePost(ctx context.Context, p *Post) error
	DeletePost(ctx context.Context, id int64) error
	SlugExists(ctx context.Context, slug string) (bool, error)

	AddComment(ctx context.Context, c *Comment) error
	ListComments(ctx context.Context, postID int64) ([]Comment, error)
}
