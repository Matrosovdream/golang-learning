package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"midigator/internal/domain"
)

// CommentRepository implements domain.CommentRepository with raw SQL. Comments
// are polymorphic (commentable_type / commentable_id) and, unlike the case
// tables, carry NO tenant_id — tenant scoping is enforced one layer up by
// verifying the subject exists within the tenant before reading or writing.
type CommentRepository struct {
	pool *pgxpool.Pool
}

func NewCommentRepository(pool *pgxpool.Pool) *CommentRepository {
	return &CommentRepository{pool: pool}
}

// commentCols is the full projection for a stand-alone comment row.
const commentCols = `id, commentable_type, commentable_id, user_id, body, is_internal, created_at, updated_at`

func scanComment(row pgx.Row) (*domain.Comment, error) {
	var c domain.Comment
	err := row.Scan(
		&c.ID, &c.CommentableType, &c.CommentableID, &c.UserID,
		&c.Body, &c.IsInternal, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *CommentRepository) Create(ctx context.Context, c *domain.Comment) (*domain.Comment, error) {
	const q = `INSERT INTO comments (commentable_type, commentable_id, user_id, body, is_internal)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`
	err := r.pool.QueryRow(ctx, q,
		c.CommentableType, c.CommentableID, c.UserID, c.Body, c.IsInternal,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert comment: %w", mapDBError(err))
	}
	return c, nil
}

func (r *CommentRepository) ListFor(ctx context.Context, subjectType string, subjectID int64) ([]domain.Comment, error) {
	const q = `SELECT c.id, c.commentable_type, c.commentable_id, c.user_id, COALESCE(u.name,''),
			c.body, c.is_internal, c.created_at, c.updated_at
		FROM comments c
		LEFT JOIN users u ON u.id = c.user_id
		WHERE c.commentable_type = $1 AND c.commentable_id = $2
		ORDER BY c.created_at`
	rows, err := r.pool.Query(ctx, q, subjectType, subjectID)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()

	var out []domain.Comment
	for rows.Next() {
		var c domain.Comment
		if err := rows.Scan(
			&c.ID, &c.CommentableType, &c.CommentableID, &c.UserID, &c.UserName,
			&c.Body, &c.IsInternal, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *CommentRepository) GetByID(ctx context.Context, id int64) (*domain.Comment, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+commentCols+` FROM comments WHERE id = $1`, id)
	c, err := scanComment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.NotFound("comment")
	}
	if err != nil {
		return nil, fmt.Errorf("get comment: %w", err)
	}
	return c, nil
}

func (r *CommentRepository) Delete(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM comments WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("comment")
	}
	return nil
}
