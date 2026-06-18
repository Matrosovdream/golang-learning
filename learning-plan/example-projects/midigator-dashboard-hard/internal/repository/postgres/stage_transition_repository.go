package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"midigator/internal/domain"
)

// StageTransitionRepository reads a case's workflow history.
type StageTransitionRepository struct {
	pool *pgxpool.Pool
}

func NewStageTransitionRepository(pool *pgxpool.Pool) *StageTransitionRepository {
	return &StageTransitionRepository{pool: pool}
}

func (r *StageTransitionRepository) ListFor(ctx context.Context, trackableType domain.CaseType, trackableID int64) ([]domain.StageTransition, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT st.id, st.trackable_type, st.trackable_id, st.user_id, COALESCE(u.name,''),
		        st.from_stage, st.to_stage, COALESCE(st.notes,''), st.created_at
		 FROM stage_transitions st
		 LEFT JOIN users u ON u.id = st.user_id
		 WHERE st.trackable_type = $1 AND st.trackable_id = $2
		 ORDER BY st.created_at`, string(trackableType), trackableID)
	if err != nil {
		return nil, fmt.Errorf("list stage transitions: %w", err)
	}
	defer rows.Close()
	var out []domain.StageTransition
	for rows.Next() {
		var t domain.StageTransition
		var tt string
		if err := rows.Scan(&t.ID, &tt, &t.TrackableID, &t.UserID, &t.UserName, &t.FromStage, &t.ToStage, &t.Notes, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.TrackableType = domain.CaseType(tt)
		out = append(out, t)
	}
	return out, rows.Err()
}
