// Package router wires every HTTP route to its handler. Tenant-plane routes
// live under /api/v1, the platform plane under /api/v1/platform, and inbound
// webhooks under /rest/v1. Public routes (login, webhook, health) sit outside
// the JWT-authenticated subtree.
package router

import (
	"net/http"

	"midigator/internal/handler"
	"midigator/internal/middleware"
)

// Deps bundles the authenticator and every handler the router mounts.
type Deps struct {
	Authn   *middleware.Authenticator
	Auth    *handler.AuthHandler
	Webhook *handler.WebhookHandler

	// Typed case handlers (show/update) + generic case handlers (index/stage/assign/hide/resolve).
	Chargeback      *handler.ChargebackHandler
	Prevention      *handler.PreventionHandler
	OrderValidation *handler.OrderValidationHandler
	Rdr             *handler.RdrHandler

	ChargebackCase      *handler.CaseHandler
	PreventionCase      *handler.CaseHandler
	OrderValidationCase *handler.CaseHandler
	RdrCase             *handler.CaseHandler

	Order        *handler.OrderHandler
	Comment      *handler.CommentHandler
	Evidence     *handler.EvidenceHandler
	Notification *handler.NotificationHandler

	TenantUser  *handler.TenantUserHandler
	Dashboard   *handler.DashboardHandler
	Manager     *handler.ManagerHandler
	Email       *handler.EmailHandler
	Export      *handler.ExportHandler
	Search      *handler.SearchHandler
	ActivityLog *handler.ActivityLogHandler

	// Platform plane.
	PlatformTenant      *handler.PlatformTenantHandler
	PlatformUser        *handler.PlatformUserHandler
	PlatformRight       *handler.PlatformRightHandler
	PlatformSetting     *handler.PlatformSettingHandler
	PlatformOverview    *handler.PlatformOverviewHandler
	PlatformIntegration *handler.PlatformIntegrationHandler
	PlatformWebhook     *handler.PlatformWebhookHandler
	Impersonation       *handler.ImpersonationHandler
	PlatformEmail       *handler.PlatformEmailHandler
	PlatformActivity    *handler.PlatformActivityHandler
}

var (
	rr = middleware.RequireRight
	pa = middleware.RequirePlatformAdmin
)

// New builds the full HTTP handler with the middleware chain applied.
func New(d Deps) http.Handler {
	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", health)
	root.HandleFunc("GET /up", health)

	// ---- public (no JWT) ----
	root.HandleFunc("POST /api/v1/auth/login", d.Auth.Login)
	root.HandleFunc("POST /api/v1/auth/login-pin", d.Auth.LoginPIN)
	root.HandleFunc("POST /rest/v1/webhooks/midigator", d.Webhook.Handle)

	// ---- protected subtree ----
	p := http.NewServeMux()
	p.HandleFunc("GET /api/v1/auth/me", d.Auth.Me)
	p.HandleFunc("POST /api/v1/auth/logout", d.Auth.Logout)

	mountChargebacks(p, d)
	mountPreventions(p, d)
	mountOrderValidations(p, d)
	mountRdr(p, d)
	mountOrders(p, d)
	mountComments(p, d)
	mountEvidence(p, d)
	mountNotifications(p, d)
	mountTenantExtras(p, d)
	mountManager(p, d)
	mountPlatform(p, d)

	root.Handle("/api/v1/", d.Authn.Authenticate(p))
	return middleware.Chain(root, middleware.RequestID, middleware.Logger, middleware.Recover)
}

func mountChargebacks(p *http.ServeMux, d Deps) {
	p.HandleFunc("GET /api/v1/chargebacks", rr("chargebacks.view", d.ChargebackCase.Index))
	p.HandleFunc("POST /api/v1/chargebacks/hide/bulk", rr("chargebacks.hide", d.Chargeback.BulkHide))
	p.HandleFunc("GET /api/v1/chargebacks/{id}", rr("chargebacks.view", d.Chargeback.Show))
	p.HandleFunc("PATCH /api/v1/chargebacks/{id}", rr("chargebacks.edit", d.Chargeback.Update))
	p.HandleFunc("POST /api/v1/chargebacks/{id}/stage", rr("chargebacks.stage_change", d.ChargebackCase.ChangeStage))
	p.HandleFunc("POST /api/v1/chargebacks/{id}/assign", rr("chargebacks.assign", d.ChargebackCase.Assign))
	p.HandleFunc("POST /api/v1/chargebacks/{id}/hide", rr("chargebacks.hide", d.ChargebackCase.Hide))
}

func mountPreventions(p *http.ServeMux, d Deps) {
	p.HandleFunc("GET /api/v1/preventions", rr("preventions.view", d.PreventionCase.Index))
	p.HandleFunc("GET /api/v1/preventions/{id}", rr("preventions.view", d.Prevention.Show))
	p.HandleFunc("PATCH /api/v1/preventions/{id}", rr("preventions.edit", d.Prevention.Update))
	p.HandleFunc("POST /api/v1/preventions/{id}/stage", rr("preventions.stage_change", d.PreventionCase.ChangeStage))
	p.HandleFunc("POST /api/v1/preventions/{id}/assign", rr("preventions.assign", d.PreventionCase.Assign))
	p.HandleFunc("POST /api/v1/preventions/{id}/hide", rr("preventions.hide", d.PreventionCase.Hide))
	p.HandleFunc("POST /api/v1/preventions/{id}/resolve", rr("preventions.resolve", d.PreventionCase.Resolve))
}

func mountOrderValidations(p *http.ServeMux, d Deps) {
	p.HandleFunc("GET /api/v1/order-validations", rr("order_validations.view", d.OrderValidationCase.Index))
	p.HandleFunc("GET /api/v1/order-validations/{id}", rr("order_validations.view", d.OrderValidation.Show))
	p.HandleFunc("PATCH /api/v1/order-validations/{id}", rr("order_validations.edit", d.OrderValidation.Update))
	p.HandleFunc("POST /api/v1/order-validations/{id}/stage", rr("order_validations.stage_change", d.OrderValidationCase.ChangeStage))
	p.HandleFunc("POST /api/v1/order-validations/{id}/assign", rr("order_validations.assign", d.OrderValidationCase.Assign))
	p.HandleFunc("POST /api/v1/order-validations/{id}/hide", rr("order_validations.hide", d.OrderValidationCase.Hide))
}

func mountRdr(p *http.ServeMux, d Deps) {
	p.HandleFunc("GET /api/v1/rdr-cases", rr("rdr.view", d.RdrCase.Index))
	p.HandleFunc("GET /api/v1/rdr-cases/{id}", rr("rdr.view", d.Rdr.Show))
	p.HandleFunc("PATCH /api/v1/rdr-cases/{id}", rr("rdr.edit", d.Rdr.Update))
	p.HandleFunc("POST /api/v1/rdr-cases/{id}/stage", rr("rdr.stage_change", d.RdrCase.ChangeStage))
	p.HandleFunc("POST /api/v1/rdr-cases/{id}/assign", rr("rdr.assign", d.RdrCase.Assign))
	p.HandleFunc("POST /api/v1/rdr-cases/{id}/hide", rr("rdr.hide", d.RdrCase.Hide))
	p.HandleFunc("POST /api/v1/rdr-cases/{id}/resolve", rr("rdr.resolve", d.RdrCase.Resolve))
}

func mountOrders(p *http.ServeMux, d Deps) {
	p.HandleFunc("GET /api/v1/orders", rr("orders.view", d.Order.Index))
	p.HandleFunc("POST /api/v1/orders", rr("orders.create", d.Order.Create))
	p.HandleFunc("GET /api/v1/orders/{id}", rr("orders.view", d.Order.Show))
	p.HandleFunc("PATCH /api/v1/orders/{id}", rr("orders.edit", d.Order.Update))
	p.HandleFunc("POST /api/v1/orders/{id}/submit", rr("orders.submit", d.Order.Submit))
}

// Comments and evidence are polymorphic: {type} is a case/order kind (singular).
// They sit under literal /comments and /evidence prefixes so the {type} wildcard
// can't collide with other 3-segment routes (e.g. /platform/tenants/{id}).
func mountComments(p *http.ServeMux, d Deps) {
	p.HandleFunc("GET /api/v1/comments/{type}/{id}", rr("comments.view", d.Comment.Index))
	p.HandleFunc("POST /api/v1/comments/{type}/{id}", rr("comments.create", d.Comment.Store))
	p.HandleFunc("DELETE /api/v1/comments/{id}", rr("comments.delete", d.Comment.Destroy))
}

func mountEvidence(p *http.ServeMux, d Deps) {
	p.HandleFunc("GET /api/v1/evidence/{type}/{id}", rr("evidence.view", d.Evidence.Index))
	p.HandleFunc("POST /api/v1/evidence/{type}/{id}", rr("evidence.upload", d.Evidence.Store))
	p.HandleFunc("DELETE /api/v1/evidence/{id}", rr("evidence.delete", d.Evidence.Destroy))
}

func mountNotifications(p *http.ServeMux, d Deps) {
	p.HandleFunc("GET /api/v1/notifications", d.Notification.Index)
	p.HandleFunc("PATCH /api/v1/notifications/{id}/read", d.Notification.MarkRead)
	p.HandleFunc("POST /api/v1/notifications/read-all", d.Notification.MarkAllRead)
	p.HandleFunc("GET /api/v1/notification-settings", d.Notification.Settings)
	p.HandleFunc("PATCH /api/v1/notification-settings", d.Notification.UpdateSettings)
}

func mountTenantExtras(p *http.ServeMux, d Deps) {
	// users directory + manager scoring
	p.HandleFunc("GET /api/v1/users", rr("users.view", d.TenantUser.Index))
	p.HandleFunc("PATCH /api/v1/users/{userId}/score", rr("managers.score", d.Manager.UpdateScore))

	// dashboard
	p.HandleFunc("GET /api/v1/dashboard/summary", rr("dashboard.view", d.Dashboard.Summary))
	p.HandleFunc("GET /api/v1/dashboard/manager-performance", rr("dashboard.manager_performance", d.Dashboard.ManagerPerformance))
	p.HandleFunc("GET /api/v1/dashboard/recent-activity", rr("dashboard.view", d.Dashboard.RecentActivity))

	// emails
	p.HandleFunc("GET /api/v1/email-templates", rr("emails.view", d.Email.ListTemplates))
	p.HandleFunc("POST /api/v1/email-templates", rr("emails.templates_manage", d.Email.CreateTemplate))
	p.HandleFunc("PATCH /api/v1/email-templates/{id}", rr("emails.templates_manage", d.Email.UpdateTemplate))
	p.HandleFunc("DELETE /api/v1/email-templates/{id}", rr("emails.templates_manage", d.Email.DeleteTemplate))
	p.HandleFunc("POST /api/v1/emails/send", rr("emails.send", d.Email.Send))
	p.HandleFunc("GET /api/v1/email-logs", rr("emails.view", d.Email.Logs))

	// export, search, activity-log
	p.HandleFunc("GET /api/v1/export/{type}", rr("export.run", d.Export.CSV))
	p.HandleFunc("GET /api/v1/search", d.Search.Index)
	p.HandleFunc("GET /api/v1/activity-log", rr("activity_log.view", d.ActivityLog.Index))
}

func mountManager(p *http.ServeMux, d Deps) {
	p.HandleFunc("GET /api/v1/manager/home", rr("dashboard.view", d.Manager.Home))
	p.HandleFunc("GET /api/v1/manager/team", rr("managers.view_profiles", d.Manager.Team))
	p.HandleFunc("GET /api/v1/manager/assignments", rr("chargebacks.assign", d.Manager.Assignments))
	p.HandleFunc("POST /api/v1/manager/assignments/{kind}/{id}", rr("chargebacks.assign", d.Manager.Assign))
	p.HandleFunc("GET /api/v1/manager/approvals", rr("chargebacks.stage_change", d.Manager.Approvals))
	p.HandleFunc("GET /api/v1/manager/activity", rr("activity_log.view_all", d.Manager.Activity))
}

func mountPlatform(p *http.ServeMux, d Deps) {
	// tenants
	p.HandleFunc("GET /api/v1/platform/tenants", pa(d.PlatformTenant.Index))
	p.HandleFunc("POST /api/v1/platform/tenants", pa(d.PlatformTenant.Create))
	p.HandleFunc("POST /api/v1/platform/tenants/test-connection", pa(d.PlatformTenant.TestConnection))
	p.HandleFunc("GET /api/v1/platform/tenants/{id}", pa(d.PlatformTenant.Show))
	p.HandleFunc("PATCH /api/v1/platform/tenants/{id}", pa(d.PlatformTenant.Update))
	p.HandleFunc("DELETE /api/v1/platform/tenants/{id}", pa(d.PlatformTenant.Destroy))
	p.HandleFunc("GET /api/v1/platform/tenants/{id}/overview", pa(d.PlatformTenant.Overview))
	p.HandleFunc("GET /api/v1/platform/tenants/{id}/users", pa(d.PlatformTenant.Users))
	p.HandleFunc("GET /api/v1/platform/tenants/{id}/activity", pa(d.PlatformTenant.Activity))
	p.HandleFunc("GET /api/v1/platform/tenants/{id}/webhooks", pa(d.PlatformTenant.Webhooks))
	p.HandleFunc("POST /api/v1/platform/tenants/{id}/toggle-active", pa(d.PlatformTenant.ToggleActive))

	// users
	p.HandleFunc("GET /api/v1/platform/users", pa(d.PlatformUser.Index))
	p.HandleFunc("POST /api/v1/platform/users/{id}/toggle-active", pa(d.PlatformUser.ToggleActive))
	p.HandleFunc("POST /api/v1/platform/users/{id}/toggle-platform-admin", pa(d.PlatformUser.TogglePlatformAdmin))

	// rights, settings, overview, integrations
	p.HandleFunc("GET /api/v1/platform/rights", pa(d.PlatformRight.Index))
	p.HandleFunc("GET /api/v1/platform/settings", pa(d.PlatformSetting.Index))
	p.HandleFunc("PUT /api/v1/platform/settings", pa(d.PlatformSetting.Upsert))
	p.HandleFunc("DELETE /api/v1/platform/settings", pa(d.PlatformSetting.Destroy))
	p.HandleFunc("GET /api/v1/platform/overview/summary", pa(d.PlatformOverview.Summary))
	p.HandleFunc("GET /api/v1/platform/integrations/health", pa(d.PlatformIntegration.Health))

	// webhook logs
	p.HandleFunc("GET /api/v1/platform/webhooks", pa(d.PlatformWebhook.Index))
	p.HandleFunc("GET /api/v1/platform/webhooks/{id}", pa(d.PlatformWebhook.Show))

	// impersonation: start requires platform admin; stop is for the acting session.
	p.HandleFunc("POST /api/v1/platform/impersonate/stop", d.Impersonation.Stop)
	p.HandleFunc("POST /api/v1/platform/impersonate/{userId}", pa(d.Impersonation.Start))

	// platform emails + activity
	p.HandleFunc("GET /api/v1/platform/emails/templates", pa(d.PlatformEmail.TemplatesIndex))
	p.HandleFunc("POST /api/v1/platform/emails/templates", pa(d.PlatformEmail.TemplateCreate))
	p.HandleFunc("GET /api/v1/platform/emails/templates/{id}", pa(d.PlatformEmail.TemplateShow))
	p.HandleFunc("PATCH /api/v1/platform/emails/templates/{id}", pa(d.PlatformEmail.TemplateUpdate))
	p.HandleFunc("DELETE /api/v1/platform/emails/templates/{id}", pa(d.PlatformEmail.TemplateDestroy))
	p.HandleFunc("GET /api/v1/platform/emails/logs", pa(d.PlatformEmail.LogsIndex))
	p.HandleFunc("GET /api/v1/platform/emails/logs/{id}", pa(d.PlatformEmail.LogShow))
	p.HandleFunc("GET /api/v1/platform/activity", pa(d.PlatformActivity.Index))
}

func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}
