package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"blogapi/internal/domain"
)

// --- GORM persistence models (kept separate from the domain entities) ---

type post struct {
	ID           int64     `gorm:"primaryKey"`
	Title        string    `gorm:"size:200;not null"`
	Slug         string    `gorm:"size:220;not null;uniqueIndex"`
	Body         string    `gorm:"not null"`
	Published    bool      `gorm:"not null;default:false;index"`
	Tags         []tag     `gorm:"many2many:post_tags;"`
	Comments     []comment `gorm:"foreignKey:PostID;constraint:OnDelete:CASCADE;"`
	CommentCount int64     `gorm:"->"` // read-only, filled by a subquery on list
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (post) TableName() string { return "posts" }

type tag struct {
	ID   int64  `gorm:"primaryKey"`
	Name string `gorm:"size:50;not null;uniqueIndex"`
}

func (tag) TableName() string { return "tags" }

type comment struct {
	ID        int64  `gorm:"primaryKey"`
	PostID    int64  `gorm:"not null;index"`
	Author    string `gorm:"size:100;not null"`
	Body      string `gorm:"not null"`
	CreatedAt time.Time
}

func (comment) TableName() string { return "comments" }

// AutoMigrate creates/updates the posts, tags, comments and join tables.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&post{}, &tag{}, &comment{})
}

// PostRepository implements domain.PostRepository with GORM.
type PostRepository struct {
	db *gorm.DB
}

func NewPostRepository(db *gorm.DB) *PostRepository {
	return &PostRepository{db: db}
}

func (r *PostRepository) CreatePost(ctx context.Context, p *domain.Post) error {
	tags, err := r.resolveTags(ctx, p.Tags)
	if err != nil {
		return err
	}
	m := post{
		Title:     p.Title,
		Slug:      p.Slug,
		Body:      p.Body,
		Published: p.Published,
		Tags:      tags,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return fmt.Errorf("create post: %w", err)
	}
	return r.reloadPost(ctx, p, m.ID)
}

func (r *PostRepository) GetPostByID(ctx context.Context, id int64) (*domain.Post, error) {
	var m post
	err := r.db.WithContext(ctx).
		Preload("Tags").
		Preload("Comments", func(db *gorm.DB) *gorm.DB { return db.Order("comments.created_at ASC") }).
		First(&m, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrPostNotFound
		}
		return nil, fmt.Errorf("get post: %w", err)
	}
	m.CommentCount = int64(len(m.Comments))
	p := toDomainPost(m)
	return &p, nil
}

func (r *PostRepository) ListPosts(ctx context.Context, f domain.PostFilter) ([]domain.Post, int64, error) {
	var total int64
	if err := r.filtered(ctx, f).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count posts: %w", err)
	}

	offset := (f.Page - 1) * f.PageSize
	var models []post
	err := r.filtered(ctx, f).
		Select("posts.*, (SELECT count(*) FROM comments WHERE comments.post_id = posts.id) AS comment_count").
		Preload("Tags").
		Order("posts.created_at DESC").
		Limit(f.PageSize).
		Offset(offset).
		Find(&models).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list posts: %w", err)
	}

	posts := make([]domain.Post, len(models))
	for i, m := range models {
		posts[i] = toDomainPost(m)
	}
	return posts, total, nil
}

// filtered builds a fresh query with the search/tag/published conditions
// applied. It is called separately for the count and the page so the two
// statements never share mutated state.
func (r *PostRepository) filtered(ctx context.Context, f domain.PostFilter) *gorm.DB {
	q := r.db.WithContext(ctx).Model(&post{})
	if f.Search != "" {
		q = q.Where("posts.title ILIKE ?", "%"+f.Search+"%")
	}
	if f.Published != nil {
		q = q.Where("posts.published = ?", *f.Published)
	}
	if f.Tag != "" {
		sub := r.db.Table("post_tags").
			Select("post_tags.post_id").
			Joins("JOIN tags ON tags.id = post_tags.tag_id").
			Where("tags.name = ?", strings.ToLower(strings.TrimSpace(f.Tag)))
		q = q.Where("posts.id IN (?)", sub)
	}
	return q
}

func (r *PostRepository) UpdatePost(ctx context.Context, p *domain.Post) error {
	tags, err := r.resolveTags(ctx, p.Tags)
	if err != nil {
		return err
	}
	// The slug is immutable after creation, so we update content only and then
	// replace the tag associations — both inside one transaction.
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&post{}).Where("id = ?", p.ID).Updates(map[string]any{
			"title":     p.Title,
			"body":      p.Body,
			"published": p.Published,
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return domain.ErrPostNotFound
		}
		return tx.Model(&post{ID: p.ID}).Association("Tags").Replace(tags)
	})
	if err != nil {
		if errors.Is(err, domain.ErrPostNotFound) {
			return err
		}
		return fmt.Errorf("update post: %w", err)
	}
	return r.reloadPost(ctx, p, p.ID)
}

func (r *PostRepository) DeletePost(ctx context.Context, id int64) error {
	// clause.Associations also clears the post_tags join rows; the comments FK
	// cascades at the database level.
	res := r.db.WithContext(ctx).Select(clause.Associations).Delete(&post{ID: id})
	if res.Error != nil {
		return fmt.Errorf("delete post: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrPostNotFound
	}
	return nil
}

func (r *PostRepository) SlugExists(ctx context.Context, slug string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&post{}).Where("slug = ?", slug).Count(&count).Error; err != nil {
		return false, fmt.Errorf("slug exists: %w", err)
	}
	return count > 0, nil
}

func (r *PostRepository) AddComment(ctx context.Context, c *domain.Comment) error {
	var count int64
	if err := r.db.WithContext(ctx).Model(&post{}).Where("id = ?", c.PostID).Count(&count).Error; err != nil {
		return fmt.Errorf("check post: %w", err)
	}
	if count == 0 {
		return domain.ErrPostNotFound
	}
	m := comment{PostID: c.PostID, Author: c.Author, Body: c.Body}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return fmt.Errorf("add comment: %w", err)
	}
	*c = toDomainComment(m)
	return nil
}

func (r *PostRepository) ListComments(ctx context.Context, postID int64) ([]domain.Comment, error) {
	var models []comment
	err := r.db.WithContext(ctx).Where("post_id = ?", postID).Order("created_at ASC").Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	out := make([]domain.Comment, len(models))
	for i, m := range models {
		out[i] = toDomainComment(m)
	}
	return out, nil
}

// resolveTags normalises tag names and upserts each one, returning the rows so
// they can be associated with a post (no duplicate tags are ever created).
func (r *PostRepository) resolveTags(ctx context.Context, names []string) ([]tag, error) {
	seen := map[string]bool{}
	var tags []tag
	for _, n := range names {
		name := strings.ToLower(strings.TrimSpace(n))
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		var t tag
		if err := r.db.WithContext(ctx).Where(tag{Name: name}).FirstOrCreate(&t).Error; err != nil {
			return nil, fmt.Errorf("resolve tag %q: %w", name, err)
		}
		tags = append(tags, t)
	}
	return tags, nil
}

// reloadPost refreshes *p with the full row (tags, comments, count).
func (r *PostRepository) reloadPost(ctx context.Context, p *domain.Post, id int64) error {
	fresh, err := r.GetPostByID(ctx, id)
	if err != nil {
		return err
	}
	*p = *fresh
	return nil
}

func toDomainPost(m post) domain.Post {
	tags := make([]string, len(m.Tags))
	for i, t := range m.Tags {
		tags[i] = t.Name
	}
	comments := make([]domain.Comment, len(m.Comments))
	for i, c := range m.Comments {
		comments[i] = toDomainComment(c)
	}
	return domain.Post{
		ID:           m.ID,
		Title:        m.Title,
		Slug:         m.Slug,
		Body:         m.Body,
		Published:    m.Published,
		Tags:         tags,
		Comments:     comments,
		CommentCount: m.CommentCount,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

func toDomainComment(m comment) domain.Comment {
	return domain.Comment{
		ID:        m.ID,
		PostID:    m.PostID,
		Author:    m.Author,
		Body:      m.Body,
		CreatedAt: m.CreatedAt,
	}
}
