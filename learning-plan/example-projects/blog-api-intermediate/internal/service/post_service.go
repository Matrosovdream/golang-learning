package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"blogapi/internal/domain"
)

const (
	defaultPageSize = 10
	maxPageSize     = 50
)

// PostService holds the business rules: validation, slug generation, paging.
type PostService struct {
	repo domain.PostRepository
}

func NewPostService(repo domain.PostRepository) *PostService {
	return &PostService{repo: repo}
}

// PostInput carries the raw values for creating/updating a post.
type PostInput struct {
	Title     string
	Body      string
	Published bool
	Tags      []string
}

// CommentInput carries the raw values for a new comment.
type CommentInput struct {
	Author string
	Body   string
}

func (s *PostService) CreatePost(ctx context.Context, in PostInput) (*domain.Post, error) {
	title, body, err := validatePost(in)
	if err != nil {
		return nil, err
	}
	slug, err := s.uniqueSlug(ctx, slugify(title))
	if err != nil {
		return nil, err
	}
	post := &domain.Post{
		Title:     title,
		Slug:      slug,
		Body:      body,
		Published: in.Published,
		Tags:      in.Tags,
	}
	if err := s.repo.CreatePost(ctx, post); err != nil {
		return nil, err
	}
	return post, nil
}

func (s *PostService) GetPost(ctx context.Context, id int64) (*domain.Post, error) {
	return s.repo.GetPostByID(ctx, id)
}

func (s *PostService) ListPosts(ctx context.Context, f domain.PostFilter) (*domain.Page[domain.Post], error) {
	f.Page, f.PageSize = normalisePaging(f.Page, f.PageSize)
	items, total, err := s.repo.ListPosts(ctx, f)
	if err != nil {
		return nil, err
	}
	totalPages := int((total + int64(f.PageSize) - 1) / int64(f.PageSize))
	return &domain.Page[domain.Post]{
		Items:      items,
		Page:       f.Page,
		PageSize:   f.PageSize,
		Total:      total,
		TotalPages: totalPages,
	}, nil
}

func (s *PostService) UpdatePost(ctx context.Context, id int64, in PostInput) (*domain.Post, error) {
	title, body, err := validatePost(in)
	if err != nil {
		return nil, err
	}
	post := &domain.Post{
		ID:        id,
		Title:     title,
		Body:      body,
		Published: in.Published,
		Tags:      in.Tags,
	}
	if err := s.repo.UpdatePost(ctx, post); err != nil {
		return nil, err
	}
	return post, nil
}

func (s *PostService) DeletePost(ctx context.Context, id int64) error {
	return s.repo.DeletePost(ctx, id)
}

func (s *PostService) AddComment(ctx context.Context, postID int64, in CommentInput) (*domain.Comment, error) {
	body := strings.TrimSpace(in.Body)
	if body == "" {
		return nil, domain.ValidationError{Field: "body", Message: "is required"}
	}
	author := strings.TrimSpace(in.Author)
	if author == "" {
		author = "anonymous"
	}
	comment := &domain.Comment{PostID: postID, Author: author, Body: body}
	if err := s.repo.AddComment(ctx, comment); err != nil {
		return nil, err
	}
	return comment, nil
}

func (s *PostService) ListComments(ctx context.Context, postID int64) ([]domain.Comment, error) {
	// Surface a 404 for an unknown post rather than an empty list.
	if _, err := s.repo.GetPostByID(ctx, postID); err != nil {
		return nil, err
	}
	return s.repo.ListComments(ctx, postID)
}

// uniqueSlug appends -2, -3, ... until the slug is free.
func (s *PostService) uniqueSlug(ctx context.Context, base string) (string, error) {
	slug := base
	for i := 2; i <= 100; i++ {
		exists, err := s.repo.SlugExists(ctx, slug)
		if err != nil {
			return "", err
		}
		if !exists {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	return "", errors.New("could not generate a unique slug")
}

func validatePost(in PostInput) (title, body string, err error) {
	title = strings.TrimSpace(in.Title)
	if title == "" {
		return "", "", domain.ValidationError{Field: "title", Message: "is required"}
	}
	if len(title) > 200 {
		return "", "", domain.ValidationError{Field: "title", Message: "must be at most 200 characters"}
	}
	body = strings.TrimSpace(in.Body)
	if body == "" {
		return "", "", domain.ValidationError{Field: "body", Message: "is required"}
	}
	return title, body, nil
}

func normalisePaging(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	switch {
	case size < 1:
		size = defaultPageSize
	case size > maxPageSize:
		size = maxPageSize
	}
	return page, size
}

// slugify turns a title into a URL-friendly slug: lowercase, alphanumerics
// kept, every other run collapsed to a single dash.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "post"
	}
	return out
}
