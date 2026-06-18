// Package seed loads demo data: the rights catalog, a demo tenant, the four
// system roles + a platform admin, the demo users (same emails/PINs as the
// original app), and a batch of synthetic cases so the dashboards render.
// It is idempotent — safe to run repeatedly.
package seed

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"midigator/internal/auth"
)

// Run seeds the database.
func Run(ctx context.Context, pool *pgxpool.Pool) error {
	if err := seedRights(ctx, pool); err != nil {
		return fmt.Errorf("rights: %w", err)
	}
	tenantID, err := seedTenant(ctx, pool)
	if err != nil {
		return fmt.Errorf("tenant: %w", err)
	}
	roleIDs, err := seedRoles(ctx, pool, tenantID)
	if err != nil {
		return fmt.Errorf("roles: %w", err)
	}
	userIDs, err := seedUsers(ctx, pool, tenantID, roleIDs)
	if err != nil {
		return fmt.Errorf("users: %w", err)
	}
	if err := seedCases(ctx, pool, tenantID, userIDs); err != nil {
		return fmt.Errorf("cases: %w", err)
	}
	return nil
}

// rightsCatalog mirrors the Laravel RightsSeeder: {slug, name, description}.
var rightsCatalog = [][3]string{
	{"chargebacks.view", "View chargebacks", "View chargeback list and details"},
	{"chargebacks.edit", "Edit chargebacks", "Edit chargeback custom fields"},
	{"chargebacks.assign", "Assign chargebacks", "Assign chargebacks to managers"},
	{"chargebacks.hide", "Hide chargebacks", "Hide/unhide chargebacks from managers"},
	{"chargebacks.stage_change", "Change chargeback stage", "Change chargeback workflow stage"},
	{"preventions.view", "View preventions", "View prevention alerts"},
	{"preventions.edit", "Edit preventions", "Edit prevention custom fields"},
	{"preventions.assign", "Assign preventions", "Assign prevention alerts to managers"},
	{"preventions.hide", "Hide preventions", "Hide/unhide prevention alerts"},
	{"preventions.resolve", "Resolve preventions", "Submit resolution to Midigator API"},
	{"preventions.stage_change", "Change prevention stage", "Change prevention workflow stage"},
	{"orders.view", "View orders", "View orders"},
	{"orders.create", "Create orders", "Create new orders and submit to Midigator"},
	{"orders.edit", "Edit orders", "Edit order custom fields"},
	{"orders.hide", "Hide orders", "Hide/unhide orders"},
	{"orders.submit", "Submit orders", "Submit order data to Midigator API"},
	{"order_validations.view", "View order validations", "View order validations"},
	{"order_validations.edit", "Edit order validations", "Edit order validation fields"},
	{"order_validations.hide", "Hide order validations", "Hide/unhide order validations"},
	{"order_validations.stage_change", "Change order validation stage", "Change order validation stage"},
	{"order_validations.assign", "Assign order validations", "Assign order validations to managers"},
	{"rdr.view", "View RDR cases", "View RDR cases"},
	{"rdr.edit", "Edit RDR cases", "Edit RDR case fields"},
	{"rdr.assign", "Assign RDR cases", "Assign RDR cases to managers"},
	{"rdr.hide", "Hide RDR cases", "Hide/unhide RDR cases"},
	{"rdr.resolve", "Resolve RDR cases", "Submit RDR resolution"},
	{"rdr.stage_change", "Change RDR case stage", "Change RDR case stage"},
	{"comments.view", "View comments", "View comments on cases"},
	{"comments.create", "Create comments", "Add comments to cases"},
	{"comments.delete", "Delete comments", "Delete any comment (not just own)"},
	{"emails.view", "View emails", "View email logs"},
	{"emails.send", "Send emails", "Send emails to customers"},
	{"emails.templates_manage", "Manage email templates", "Create/edit/delete email templates"},
	{"users.view", "View users", "View user list"},
	{"users.manage", "Manage users", "Create, edit, deactivate users"},
	{"users.invite", "Invite users", "Invite new users to tenant"},
	{"roles.view", "View roles", "View roles and their rights"},
	{"roles.manage", "Manage roles", "Create, edit, delete custom roles and assign rights"},
	{"managers.score", "Score managers", "Set manager performance scores"},
	{"managers.view_profiles", "View manager profiles", "View manager profiles and stats"},
	{"dashboard.view", "View dashboard", "View dashboard stats and charts"},
	{"dashboard.manager_performance", "View manager performance", "View manager leaderboard and performance"},
	{"settings.view", "View settings", "View tenant settings"},
	{"settings.manage", "Manage settings", "Edit tenant settings, API keys"},
	{"activity_log.view", "View own activity log", "View own activity log"},
	{"activity_log.view_all", "View all activity logs", "View all users' activity logs"},
	{"notifications.manage", "Manage notifications", "Manage own notification preferences"},
	{"evidence.view", "View evidence", "View evidence files attached to cases"},
	{"evidence.upload", "Upload evidence", "Upload evidence files to cases"},
	{"evidence.delete", "Delete evidence", "Delete evidence files from cases"},
	{"export.run", "Run exports", "Export CSV extracts for tenant data"},
}

func seedRights(ctx context.Context, pool *pgxpool.Pool) error {
	for _, r := range rightsCatalog {
		slug, name, desc := r[0], r[1], r[2]
		group := slug[:indexDot(slug)]
		if _, err := pool.Exec(ctx,
			`INSERT INTO rights (slug, name, "group", description) VALUES ($1,$2,$3,$4)
			 ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name, "group" = EXCLUDED."group", description = EXCLUDED.description`,
			slug, name, group, desc); err != nil {
			return err
		}
	}
	return nil
}

func indexDot(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return i
		}
	}
	return len(s)
}

func seedTenant(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var id int64
	err := pool.QueryRow(ctx,
		`INSERT INTO tenants (name, slug, is_active, midigator_webhook_username, midigator_webhook_password)
		 VALUES ('Demo Tenant', 'demo', TRUE, 'midigator', 'webhook-secret')
		 ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
		 RETURNING id`).Scan(&id)
	return id, err
}

var roleRights = map[string][]string{
	"tenant-admin": nil, // all rights (filled below)
	"manager": {
		"chargebacks.view", "chargebacks.edit", "chargebacks.stage_change", "chargebacks.assign",
		"preventions.view", "preventions.edit", "preventions.resolve", "preventions.stage_change", "preventions.assign",
		"orders.view", "orders.create", "orders.edit", "orders.submit",
		"order_validations.view", "order_validations.edit", "order_validations.stage_change",
		"rdr.view", "rdr.edit", "rdr.stage_change", "rdr.assign",
		"comments.view", "comments.create", "emails.view", "emails.send",
		"dashboard.view", "activity_log.view", "notifications.manage", "export.run",
		"users.view", "evidence.view", "evidence.upload", "evidence.delete",
	},
	"analyst": {
		"chargebacks.view", "chargebacks.edit", "preventions.view", "preventions.edit",
		"orders.view", "order_validations.view", "order_validations.edit",
		"rdr.view", "rdr.edit", "comments.view", "comments.create",
		"dashboard.view", "export.run", "evidence.view", "evidence.upload",
	},
	"viewer": {
		"chargebacks.view", "preventions.view", "orders.view", "order_validations.view",
		"rdr.view", "comments.view", "dashboard.view", "evidence.view",
	},
}

var roleMeta = []struct {
	slug, name, desc string
}{
	{"tenant-admin", "Tenant Admin", "Built-in role with all rights. Cannot be deleted."},
	{"manager", "Manager", "Built-in role with limited rights. Cannot be deleted."},
	{"analyst", "Analyst", "Works cases but cannot assign or change stages owned by managers."},
	{"viewer", "Viewer", "Read-only access to cases and dashboard."},
}

func seedRoles(ctx context.Context, pool *pgxpool.Pool, tenantID int64) (map[string]int64, error) {
	ids := map[string]int64{}
	for _, m := range roleMeta {
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO roles (tenant_id, name, slug, description, is_system) VALUES ($1,$2,$3,$4,TRUE)
			 ON CONFLICT (tenant_id, slug) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description
			 RETURNING id`, tenantID, m.name, m.slug, m.desc).Scan(&id); err != nil {
			return nil, err
		}
		ids[m.slug] = id

		slugs := roleRights[m.slug]
		if m.slug == "tenant-admin" {
			for _, r := range rightsCatalog {
				slugs = append(slugs, r[0])
			}
		}
		if _, err := pool.Exec(ctx, `DELETE FROM role_rights WHERE role_id = $1`, id); err != nil {
			return nil, err
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO role_rights (role_id, right_id) SELECT $1, id FROM rights WHERE slug = ANY($2) ON CONFLICT DO NOTHING`,
			id, slugs); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

var demoUsers = []struct {
	email, name, pin, role string
}{
	{"admin@midigator.test", "Demo Admin", "1234", "tenant-admin"},
	{"manager@midigator.test", "Demo Manager", "5678", "manager"},
	{"analyst@midigator.test", "Demo Analyst", "4321", "analyst"},
	{"viewer@midigator.test", "Demo Viewer", "8765", "viewer"},
}

func seedUsers(ctx context.Context, pool *pgxpool.Pool, tenantID int64, roleIDs map[string]int64) (map[string]int64, error) {
	pw, _ := auth.Hash("password")
	ids := map[string]int64{}
	for _, u := range demoUsers {
		pin, _ := auth.Hash(u.pin)
		var id int64
		if err := pool.QueryRow(ctx,
			`INSERT INTO users (tenant_id, email, password, name, pin, is_active, is_platform_admin)
			 VALUES ($1,$2,$3,$4,$5,TRUE,FALSE)
			 ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name, pin = EXCLUDED.pin, tenant_id = EXCLUDED.tenant_id
			 RETURNING id`, tenantID, u.email, pw, u.name, pin).Scan(&id); err != nil {
			return nil, err
		}
		ids[u.role] = id
		if rid, ok := roleIDs[u.role]; ok {
			if _, err := pool.Exec(ctx,
				`INSERT INTO user_roles (user_id, role_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, id, rid); err != nil {
				return nil, err
			}
		}
	}

	// Platform admin (no tenant).
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (tenant_id, email, password, name, pin, is_active, is_platform_admin)
		 VALUES (NULL,'platform@midigator.test',$1,'Platform Admin',$2,TRUE,TRUE)
		 ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name, pin = EXCLUDED.pin`,
		pw, mustHash("9999")); err != nil {
		return nil, err
	}
	return ids, nil
}

func mustHash(s string) string { h, _ := auth.Hash(s); return h }

// ---- synthetic cases -------------------------------------------------------

var (
	mids        = []string{"MID-1001", "MID-1002", "MID-2050", "MID-3090"}
	cardBrands  = []string{"visa", "mastercard", "amex", "discover"}
	cbStages    = []string{"new", "in_review", "action_taken", "responded", "resolved"}
	cbResults   = []string{"pending", "won", "lost", "pre_arbitration"}
	prevStages  = []string{"new", "in_review", "action_taken", "resolved"}
	prevTypes   = []string{"ethoca", "verifi", "tc40"}
	rdrStages   = []string{"new", "in_review", "resolved"}
	reasonCodes = []string{"10.4", "13.1", "4853", "4855", "12.6.1"}
)

func seedCases(ctx context.Context, pool *pgxpool.Pool, tenantID int64, userIDs map[string]int64) error {
	var existing int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM chargebacks WHERE tenant_id = $1`, tenantID).Scan(&existing); err != nil {
		return err
	}
	if existing > 0 {
		return nil // already seeded
	}

	assignees := []*int64{nil}
	if id, ok := userIDs["manager"]; ok {
		assignees = append(assignees, &id)
	}
	if id, ok := userIDs["analyst"]; ok {
		assignees = append(assignees, &id)
	}

	now := time.Now()
	for i := 0; i < 36; i++ {
		mid := pick(mids)
		amount := int64(randInt(1500, 90000))
		assigned := assignees[randInt(0, len(assignees))]
		due := now.AddDate(0, 0, randInt(-5, 20))
		if _, err := pool.Exec(ctx,
			`INSERT INTO chargebacks (tenant_id, chargeback_guid, event_guid, case_number, arn, mid, amount, currency,
				card_brand, card_first_6, card_last_4, reason_code, reason_description, chargeback_date, date_received, due_date,
				result, assigned_to, stage)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,'USD',$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
			tenantID, guid("cb"), guid("evt"), fmt.Sprintf("CASE-%05d", randInt(10000, 99999)), guid("arn"),
			mid, amount, pick(cardBrands), "411111", fmt.Sprintf("%04d", randInt(0, 9999)),
			pick(reasonCodes), "Cardholder dispute", now.AddDate(0, 0, -randInt(1, 30)), now.AddDate(0, 0, -randInt(0, 10)), due,
			pick(cbResults), assigned, pick(cbStages),
		); err != nil {
			return err
		}
	}

	for i := 0; i < 18; i++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO prevention_alerts (tenant_id, prevention_guid, event_guid, prevention_type, arn, mid, amount, currency,
				card_first_6, card_last_4, merchant_descriptor, assigned_to, stage)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,'USD','411111',$8,'DEMO STORE',$9,$10)`,
			tenantID, guid("prev"), guid("evt"), pick(prevTypes), guid("arn"), pick(mids), int64(randInt(1000, 60000)),
			fmt.Sprintf("%04d", randInt(0, 9999)), assignees[randInt(0, len(assignees))], pick(prevStages),
		); err != nil {
			return err
		}
	}

	for i := 0; i < 12; i++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO order_validations (tenant_id, order_validation_guid, event_guid, order_validation_type, amount, currency,
				card_first_6, card_last_4, arn, mid, stage)
			 VALUES ($1,$2,$3,'cvv_check',$4,'USD','411111',$5,$6,$7,$8)`,
			tenantID, guid("ov"), guid("evt"), int64(randInt(1000, 50000)), fmt.Sprintf("%04d", randInt(0, 9999)),
			guid("arn"), pick(mids), pick(prevStages),
		); err != nil {
			return err
		}
	}

	for i := 0; i < 9; i++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO rdr_cases (tenant_id, rdr_guid, event_guid, rdr_case_number, amount, currency, arn, merchant_descriptor, prevention_type, stage)
			 VALUES ($1,$2,$3,$4,$5,'USD',$6,'DEMO STORE',$7,$8)`,
			tenantID, guid("rdr"), guid("evt"), fmt.Sprintf("RDR-%05d", randInt(10000, 99999)),
			int64(randInt(1000, 70000)), guid("arn"), pick(prevTypes), pick(rdrStages),
		); err != nil {
			return err
		}
	}

	for i := 0; i < 15; i++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO orders (tenant_id, order_guid, order_id, mid, order_amount, currency, card_brand,
				card_first_6, card_last_4, email, submission_status)
			 VALUES ($1,$2,$3,$4,$5,'USD',$6,'411111',$7,$8,'pending')`,
			tenantID, guid("ord"), fmt.Sprintf("ORD-%06d", randInt(100000, 999999)), pick(mids),
			int64(randInt(2000, 120000)), pick(cardBrands), fmt.Sprintf("%04d", randInt(0, 9999)),
			fmt.Sprintf("customer%d@example.com", i),
		); err != nil {
			return err
		}
	}
	return nil
}

// ---- tiny deterministic-ish helpers ----------------------------------------

func guid(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "-" + hex.EncodeToString(b)
}

func randInt(min, max int) int {
	if max <= min {
		return min
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max-min)))
	return min + int(n.Int64())
}

func pick[T any](s []T) T {
	return s[randInt(0, len(s))]
}
