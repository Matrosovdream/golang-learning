package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"midigator/internal/domain"
)

// EvidenceRepository implements domain.EvidenceRepository with raw SQL. Evidence
// files are polymorphic (evidenceable_type/evidenceable_id) but, unlike
// comments, they carry a tenant_id, so every query is tenant-scoped with an
// explicit `tenant_id = $N` predicate.
type EvidenceRepository struct {
	pool *pgxpool.Pool
}

func NewEvidenceRepository(pool *pgxpool.Pool) *EvidenceRepository {
	return &EvidenceRepository{pool: pool}
}

// evidenceCols is the full projection; nullable mime_type is COALESCE'd to ” so
// it scans into a plain string. UploaderName is filled only by ListFor's join.
const evidenceCols = `id, tenant_id, evidenceable_type, evidenceable_id, uploaded_by,
	name, path, COALESCE(mime_type,''), size_bytes, created_at, updated_at`

func scanEvidence(row pgx.Row) (*domain.EvidenceFile, error) {
	var e domain.EvidenceFile
	err := row.Scan(
		&e.ID, &e.TenantID, &e.EvidenceableType, &e.EvidenceableID, &e.UploadedBy,
		&e.Name, &e.Path, &e.MimeType, &e.SizeBytes, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *EvidenceRepository) Create(ctx context.Context, e *domain.EvidenceFile) (*domain.EvidenceFile, error) {
	const q = `INSERT INTO evidence_files
		(tenant_id, evidenceable_type, evidenceable_id, uploaded_by, name, path, mime_type, size_bytes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at`
	err := r.pool.QueryRow(ctx, q,
		e.TenantID, e.EvidenceableType, e.EvidenceableID, e.UploadedBy,
		e.Name, e.Path, nullable(e.MimeType), e.SizeBytes,
	).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert evidence file: %w", mapDBError(err))
	}
	return e, nil
}

// ListFor returns a subject's evidence files (newest first), joining users for
// the uploader name. Tenant-scoped.
func (r *EvidenceRepository) ListFor(ctx context.Context, tenantID int64, subjectType string, subjectID int64) ([]domain.EvidenceFile, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT e.id, e.tenant_id, e.evidenceable_type, e.evidenceable_id, e.uploaded_by, COALESCE(u.name,''),
		        e.name, e.path, COALESCE(e.mime_type,''), e.size_bytes, e.created_at, e.updated_at
		 FROM evidence_files e
		 LEFT JOIN users u ON u.id = e.uploaded_by
		 WHERE e.tenant_id = $1 AND e.evidenceable_type = $2 AND e.evidenceable_id = $3
		 ORDER BY e.created_at DESC`, tenantID, subjectType, subjectID)
	if err != nil {
		return nil, fmt.Errorf("list evidence files: %w", err)
	}
	defer rows.Close()
	var out []domain.EvidenceFile
	for rows.Next() {
		var e domain.EvidenceFile
		if err := rows.Scan(&e.ID, &e.TenantID, &e.EvidenceableType, &e.EvidenceableID, &e.UploadedBy, &e.UploaderName,
			&e.Name, &e.Path, &e.MimeType, &e.SizeBytes, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *EvidenceRepository) GetByID(ctx context.Context, tenantID, id int64) (*domain.EvidenceFile, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+evidenceCols+` FROM evidence_files WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	e, err := scanEvidence(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.NotFound("evidence file")
	}
	if err != nil {
		return nil, fmt.Errorf("get evidence file: %w", err)
	}
	return e, nil
}

func (r *EvidenceRepository) Delete(ctx context.Context, tenantID, id int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM evidence_files WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	if err != nil {
		return fmt.Errorf("delete evidence file: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("evidence file")
	}
	return nil
}
