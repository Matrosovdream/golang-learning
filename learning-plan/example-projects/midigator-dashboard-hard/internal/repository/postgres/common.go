// Package postgres holds the pgx/raw-SQL implementations of the domain
// repository interfaces. Every tenant-scoped query carries an explicit
// `tenant_id = $N` predicate — the Go analogue of the Laravel global TenantScope.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"midigator/internal/domain"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// updateByID applies a whitelisted partial update to one tenant-scoped row and
// reports whether a row was affected. Only columns present in `allowed` are
// written, so untrusted keys from a request body can never reach the SQL.
func updateByID(ctx context.Context, q querier, table string, tenantID, id int64, fields map[string]any, allowed map[string]bool) (bool, error) {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		if allowed[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys) // deterministic SQL

	set := make([]string, 0, len(keys)+1)
	args := make([]any, 0, len(keys)+2)
	i := 1
	for _, k := range keys {
		set = append(set, fmt.Sprintf("%s = $%d", k, i))
		args = append(args, fields[k])
		i++
	}
	set = append(set, "updated_at = now()")
	args = append(args, tenantID, id)

	sql := fmt.Sprintf("UPDATE %s SET %s WHERE tenant_id = $%d AND id = $%d", table, strings.Join(set, ", "), i, i+1)
	tag, err := q.Exec(ctx, sql, args...)
	if err != nil {
		return false, fmt.Errorf("update %s: %w", table, mapDBError(err))
	}
	return tag.RowsAffected() > 0, nil
}

// mapDBError converts a unique-violation into a domain ConflictError; other
// errors pass through unchanged.
func mapDBError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.ConflictError{Message: "a record with these details already exists"}
	}
	return err
}

// existsForTenant reports whether a tenant-scoped row exists.
func existsForTenant(ctx context.Context, pool *pgxpool.Pool, table string, tenantID, id int64) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx,
		fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE tenant_id = $1 AND id = $2)", table),
		tenantID, id,
	).Scan(&exists)
	return exists, err
}

// nullable converts an empty string to a SQL NULL, else passes the value.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// changeStageTx moves a case to a new stage and records a stage_transition row
// in one transaction (with a FOR UPDATE lock). Shared by every case repository.
func changeStageTx(ctx context.Context, pool *pgxpool.Pool, table string, caseType domain.CaseType, tenantID, id int64, toStage string, userID int64, notes string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var from string
	err = tx.QueryRow(ctx, fmt.Sprintf("SELECT stage FROM %s WHERE tenant_id = $1 AND id = $2 FOR UPDATE", table), tenantID, id).Scan(&from)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NotFound(string(caseType))
	}
	if err != nil {
		return fmt.Errorf("lock %s: %w", table, err)
	}
	if from == toStage {
		return tx.Commit(ctx) // no-op move; nothing to record
	}

	if _, err := tx.Exec(ctx, fmt.Sprintf("UPDATE %s SET stage = $1, updated_at = now() WHERE id = $2", table), toStage, id); err != nil {
		return fmt.Errorf("update stage: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO stage_transitions (trackable_type, trackable_id, user_id, from_stage, to_stage, notes)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		string(caseType), id, userID, from, toStage, nullable(notes)); err != nil {
		return fmt.Errorf("record transition: %w", err)
	}
	return tx.Commit(ctx)
}

// countByStage returns stage -> count for a tenant's rows in a case table.
func countByStage(ctx context.Context, pool *pgxpool.Pool, table string, tenantID int64) (map[string]int, error) {
	rows, err := pool.Query(ctx, fmt.Sprintf("SELECT stage, COUNT(*) FROM %s WHERE tenant_id = $1 GROUP BY stage", table), tenantID)
	if err != nil {
		return nil, fmt.Errorf("count by stage %s: %w", table, err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var stage string
		var n int
		if err := rows.Scan(&stage, &n); err != nil {
			return nil, err
		}
		out[stage] = n
	}
	return out, rows.Err()
}

// orderClause returns a safe `ORDER BY <col> <dir>` from a normalized CaseFilter.
func orderClause(f domain.CaseFilter) string {
	col := "created_at"
	switch f.SortBy {
	case "updated_at":
		col = "updated_at"
	case "amount":
		col = "amount"
	}
	dir := "DESC"
	if f.SortDir == "asc" {
		dir = "ASC"
	}
	return "ORDER BY " + col + " " + dir
}
