package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"midigator/internal/domain"
)

// ManagerProfileRepository implements domain.ManagerProfileRepository with raw
// SQL. manager_profiles has no tenant_id, so tenant scoping is done by joining
// users. score is nullable numeric (*float64), notes is nullable text COALESCE'd
// to ”, assigned_mids is JSONB.
type ManagerProfileRepository struct {
	pool *pgxpool.Pool
}

func NewManagerProfileRepository(pool *pgxpool.Pool) *ManagerProfileRepository {
	return &ManagerProfileRepository{pool: pool}
}

// Get returns a user's profile, or a zero-value profile {UserID: userID} (and no
// error) when the user has no profile row yet.
func (r *ManagerProfileRepository) Get(ctx context.Context, userID int64) (*domain.ManagerProfile, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT user_id, score, COALESCE(notes,''), assigned_mids, created_at, updated_at
		 FROM manager_profiles WHERE user_id = $1`, userID)
	var p domain.ManagerProfile
	err := row.Scan(&p.UserID, &p.Score, &p.Notes, &p.AssignedMids, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return &domain.ManagerProfile{UserID: userID}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get manager profile: %w", err)
	}
	return &p, nil
}

// SetScore upserts a profile's score and notes, then returns the fresh profile.
func (r *ManagerProfileRepository) SetScore(ctx context.Context, userID int64, score *float64, notes string) (*domain.ManagerProfile, error) {
	const q = `INSERT INTO manager_profiles (user_id, score, notes)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE
		SET score = EXCLUDED.score, notes = EXCLUDED.notes, updated_at = now()`
	if _, err := r.pool.Exec(ctx, q, userID, score, nullable(notes)); err != nil {
		return nil, fmt.Errorf("set manager score: %w", mapDBError(err))
	}
	return r.Get(ctx, userID)
}

// ListByTenant returns a profile for every user in the tenant (LEFT JOIN), so
// users without a profile yield null score/notes/assigned_mids/timestamps.
func (r *ManagerProfileRepository) ListByTenant(ctx context.Context, tenantID int64) ([]domain.ManagerProfile, error) {
	const q = `SELECT u.id, u.name, u.email,
			mp.score, mp.notes, mp.assigned_mids, mp.created_at, mp.updated_at
		FROM users u
		LEFT JOIN manager_profiles mp ON mp.user_id = u.id
		WHERE u.tenant_id = $1
		ORDER BY u.name ASC`
	rows, err := r.pool.Query(ctx, q, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list manager profiles: %w", err)
	}
	defer rows.Close()

	var out []domain.ManagerProfile
	for rows.Next() {
		var p domain.ManagerProfile
		var notes *string
		var assigned []byte
		var createdAt, updatedAt *time.Time
		if err := rows.Scan(&p.UserID, &p.UserName, &p.UserEmail,
			&p.Score, &notes, &assigned, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if notes != nil {
			p.Notes = *notes
		}
		if assigned != nil {
			p.AssignedMids = json.RawMessage(assigned)
		}
		if createdAt != nil {
			p.CreatedAt = *createdAt
		}
		if updatedAt != nil {
			p.UpdatedAt = *updatedAt
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
