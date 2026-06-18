package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"midigator/internal/domain"
)

// RightRepository implements domain.RightRepository (the global rights catalog).
type RightRepository struct {
	pool *pgxpool.Pool
}

func NewRightRepository(pool *pgxpool.Pool) *RightRepository {
	return &RightRepository{pool: pool}
}

func (r *RightRepository) List(ctx context.Context) ([]domain.Right, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name, slug, "group", COALESCE(description,''), created_at FROM rights ORDER BY "group", slug`)
	if err != nil {
		return nil, fmt.Errorf("list rights: %w", err)
	}
	defer rows.Close()
	var out []domain.Right
	for rows.Next() {
		var rt domain.Right
		if err := rows.Scan(&rt.ID, &rt.Name, &rt.Slug, &rt.Group, &rt.Description, &rt.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rt)
	}
	return out, rows.Err()
}

func (r *RightRepository) IDsForSlugs(ctx context.Context, slugs []string) ([]int64, error) {
	if len(slugs) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT id FROM rights WHERE slug = ANY($1)`, slugs)
	if err != nil {
		return nil, fmt.Errorf("rights ids: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *RightRepository) Upsert(ctx context.Context, slug, name, group, description string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO rights (slug, name, "group", description) VALUES ($1,$2,$3,$4)
		 ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name, "group" = EXCLUDED."group", description = EXCLUDED.description`,
		slug, name, group, nullable(description))
	return err
}

// RoleRepository implements domain.RoleRepository.
type RoleRepository struct {
	pool *pgxpool.Pool
}

func NewRoleRepository(pool *pgxpool.Pool) *RoleRepository {
	return &RoleRepository{pool: pool}
}

func (r *RoleRepository) Create(ctx context.Context, in domain.RoleInput) (*domain.Role, error) {
	var id int64
	err := r.pool.QueryRow(ctx,
		`INSERT INTO roles (tenant_id, name, slug, description, is_system) VALUES ($1,$2,$3,$4,FALSE) RETURNING id`,
		in.TenantID, in.Name, in.Slug, nullable(in.Description)).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("create role: %w", mapDBError(err))
	}
	if err := r.SyncRights(ctx, id, in.RightSlugs); err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

func (r *RoleRepository) Update(ctx context.Context, id int64, in domain.RoleInput) (*domain.Role, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE roles SET name = $1, description = $2, updated_at = now() WHERE id = $3`,
		in.Name, nullable(in.Description), id)
	if err != nil {
		return nil, fmt.Errorf("update role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, domain.NotFound("role")
	}
	if err := r.SyncRights(ctx, id, in.RightSlugs); err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

func (r *RoleRepository) Delete(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM roles WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("role")
	}
	return nil
}

func (r *RoleRepository) GetByID(ctx context.Context, id int64) (*domain.Role, error) {
	role, err := r.scanRoleRow(r.pool.QueryRow(ctx, roleSelect+` WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.NotFound("role")
	}
	if err != nil {
		return nil, err
	}
	if role.Rights, err = r.rightsForRole(ctx, role.ID); err != nil {
		return nil, err
	}
	return role, nil
}

func (r *RoleRepository) GetBySlug(ctx context.Context, tenantID *int64, slug string) (*domain.Role, error) {
	var (
		role *domain.Role
		err  error
	)
	if tenantID == nil {
		role, err = r.scanRoleRow(r.pool.QueryRow(ctx, roleSelect+` WHERE tenant_id IS NULL AND slug = $1`, slug))
	} else {
		role, err = r.scanRoleRow(r.pool.QueryRow(ctx, roleSelect+` WHERE tenant_id = $1 AND slug = $2`, *tenantID, slug))
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.NotFound("role")
	}
	if err != nil {
		return nil, err
	}
	if role.Rights, err = r.rightsForRole(ctx, role.ID); err != nil {
		return nil, err
	}
	return role, nil
}

func (r *RoleRepository) ListByTenant(ctx context.Context, tenantID int64) ([]domain.Role, error) {
	rows, err := r.pool.Query(ctx, roleSelect+` WHERE tenant_id = $1 ORDER BY id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()
	var roles []domain.Role
	for rows.Next() {
		role, err := r.scanRoleRow(rows)
		if err != nil {
			return nil, err
		}
		roles = append(roles, *role)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range roles {
		if roles[i].Rights, err = r.rightsForRole(ctx, roles[i].ID); err != nil {
			return nil, err
		}
	}
	return roles, nil
}

const roleSelect = `SELECT id, tenant_id, name, slug, COALESCE(description,''), is_system, created_at FROM roles`

func (r *RoleRepository) scanRoleRow(row pgx.Row) (*domain.Role, error) {
	var role domain.Role
	if err := row.Scan(&role.ID, &role.TenantID, &role.Name, &role.Slug, &role.Description, &role.IsSystem, &role.CreatedAt); err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *RoleRepository) rightsForRole(ctx context.Context, roleID int64) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT ri.slug FROM rights ri JOIN role_rights rr ON rr.right_id = ri.id WHERE rr.role_id = $1 ORDER BY ri.slug`, roleID)
	if err != nil {
		return nil, fmt.Errorf("role rights: %w", err)
	}
	defer rows.Close()
	var slugs []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		slugs = append(slugs, s)
	}
	return slugs, rows.Err()
}

// SyncRights replaces a role's right set with the rights matching the slugs.
func (r *RoleRepository) SyncRights(ctx context.Context, roleID int64, rightSlugs []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM role_rights WHERE role_id = $1`, roleID); err != nil {
		return fmt.Errorf("clear role rights: %w", err)
	}
	if len(rightSlugs) > 0 {
		if _, err := tx.Exec(ctx,
			`INSERT INTO role_rights (role_id, right_id)
			 SELECT $1, id FROM rights WHERE slug = ANY($2)
			 ON CONFLICT DO NOTHING`, roleID, rightSlugs); err != nil {
			return fmt.Errorf("set role rights: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (r *RoleRepository) RightsForUser(ctx context.Context, userID int64) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT ri.slug
		 FROM rights ri
		 JOIN role_rights rr ON rr.right_id = ri.id
		 JOIN user_roles ur ON ur.role_id = rr.role_id
		 WHERE ur.user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("rights for user: %w", err)
	}
	defer rows.Close()
	var slugs []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		slugs = append(slugs, s)
	}
	return slugs, rows.Err()
}

func (r *RoleRepository) RolesForUser(ctx context.Context, userID int64) ([]domain.Role, error) {
	rows, err := r.pool.Query(ctx, roleSelect+`
		WHERE id IN (SELECT role_id FROM user_roles WHERE user_id = $1) ORDER BY id`, userID)
	if err != nil {
		return nil, fmt.Errorf("roles for user: %w", err)
	}
	defer rows.Close()
	var roles []domain.Role
	for rows.Next() {
		role, err := r.scanRoleRow(rows)
		if err != nil {
			return nil, err
		}
		roles = append(roles, *role)
	}
	return roles, rows.Err()
}

func (r *RoleRepository) AssignUser(ctx context.Context, userID, roleID int64, grantedBy *int64) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id, granted_by) VALUES ($1,$2,$3) ON CONFLICT (user_id, role_id) DO NOTHING`,
		userID, roleID, grantedBy)
	return err
}

func (r *RoleRepository) SyncUserRoles(ctx context.Context, userID int64, roleIDs []int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, userID); err != nil {
		return err
	}
	for _, rid := range roleIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, userID, rid); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
