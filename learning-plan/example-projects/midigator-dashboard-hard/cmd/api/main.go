// Command api is the HTTP server for the Midigator dashboard.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"midigator/internal/auth"
	"midigator/internal/config"
	"midigator/internal/db"
	"midigator/internal/domain"
	"midigator/internal/events"
	"midigator/internal/handler"
	"midigator/internal/middleware"
	pg "midigator/internal/repository/postgres"
	"midigator/internal/rights"
	"midigator/internal/router"
	"midigator/internal/service"
	"midigator/internal/worker"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	pool, err := db.Connect(ctx, cfg.DatabaseURL, 15)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	// ---- repositories ----
	userRepo := pg.NewUserRepository(pool)
	roleRepo := pg.NewRoleRepository(pool)
	rightRepo := pg.NewRightRepository(pool)
	tenantRepo := pg.NewTenantRepository(pool)
	settingsRepo := pg.NewSettingsRepository(pool)
	activityRepo := pg.NewActivityRepository(pool)
	transitionsRepo := pg.NewStageTransitionRepository(pool)
	chargebackRepo := pg.NewChargebackRepository(pool)
	preventionRepo := pg.NewPreventionRepository(pool)
	orderValidationRepo := pg.NewOrderValidationRepository(pool)
	rdrRepo := pg.NewRdrRepository(pool)
	orderRepo := pg.NewOrderRepository(pool)
	commentRepo := pg.NewCommentRepository(pool)
	evidenceRepo := pg.NewEvidenceRepository(pool)
	notificationRepo := pg.NewNotificationRepository(pool)
	emailTemplateRepo := pg.NewEmailTemplateRepository(pool)
	emailLogRepo := pg.NewEmailLogRepository(pool)
	webhookRepo := pg.NewWebhookRepository(pool)
	managerProfileRepo := pg.NewManagerProfileRepository(pool)
	reportingRepo := pg.NewReportingRepository(pool)

	// ---- cross-cutting ----
	bus := events.NewBus()
	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTTTL)
	rightsCache := rights.New(roleRepo.RightsForUser, 5*time.Minute)
	authn := middleware.NewAuthenticator(tokens, userRepo, rightsCache)
	enqueuer := worker.NewEnqueuer(cfg.RedisAddr)
	defer enqueuer.Close()

	// CaseAssigned events (published when a case is assigned) -> notifications.
	service.RegisterEventSubscribers(bus, notificationRepo, userRepo)

	// ---- case registry + polymorphic subject checker ----
	registry := service.NewCaseRegistry()
	registry.Register(chargebackRepo)
	registry.Register(preventionRepo)
	registry.Register(orderValidationRepo)
	registry.Register(rdrRepo)
	subjects := service.NewSubjectChecker(registry, orderRepo)

	// ---- services ----
	authSvc := service.NewAuthService(userRepo, roleRepo, tokens)
	caseSvc := service.NewCaseService(registry, bus, activityRepo)
	chargebackSvc := service.NewChargebackService(chargebackRepo, transitionsRepo, activityRepo)
	preventionSvc := service.NewPreventionService(preventionRepo, transitionsRepo, activityRepo)
	orderValidationSvc := service.NewOrderValidationService(orderValidationRepo, transitionsRepo, activityRepo)
	rdrSvc := service.NewRdrService(rdrRepo, transitionsRepo, activityRepo)
	orderSvc := service.NewOrderService(orderRepo, activityRepo)
	commentSvc := service.NewCommentService(commentRepo, subjects, activityRepo)
	evidenceSvc := service.NewEvidenceService(evidenceRepo, subjects, activityRepo)
	notificationSvc := service.NewNotificationService(notificationRepo)
	dashboardSvc := service.NewDashboardService(reportingRepo, activityRepo)
	managerSvc := service.NewManagerService(reportingRepo, managerProfileRepo, caseSvc, activityRepo)
	emailSvc := service.NewEmailService(emailTemplateRepo, emailLogRepo, activityRepo)
	exportSvc := service.NewExportService(registry, orderRepo)
	searchSvc := service.NewSearchService(registry)
	activitySvc := service.NewActivityService(activityRepo)
	webhookSvc := service.NewWebhookService(tenantRepo, webhookRepo, enqueuer)

	platformTenantSvc := service.NewPlatformTenantService(tenantRepo, userRepo, activityRepo, webhookRepo, reportingRepo)
	platformUserSvc := service.NewPlatformUserService(userRepo, activityRepo)
	platformSettingSvc := service.NewPlatformSettingService(settingsRepo)
	impersonationSvc := service.NewImpersonationService(tokens, userRepo)

	// ---- handlers / deps ----
	deps := router.Deps{
		Authn:   authn,
		Auth:    handler.NewAuthHandler(authSvc),
		Webhook: handler.NewWebhookHandler(webhookSvc),

		Chargeback:      handler.NewChargebackHandler(chargebackSvc),
		Prevention:      handler.NewPreventionHandler(preventionSvc),
		OrderValidation: handler.NewOrderValidationHandler(orderValidationSvc),
		Rdr:             handler.NewRdrHandler(rdrSvc),

		ChargebackCase:      handler.NewCaseHandler(caseSvc, domain.CaseChargeback),
		PreventionCase:      handler.NewCaseHandler(caseSvc, domain.CasePrevention),
		OrderValidationCase: handler.NewCaseHandler(caseSvc, domain.CaseOrderValidation),
		RdrCase:             handler.NewCaseHandler(caseSvc, domain.CaseRdr),

		Order:        handler.NewOrderHandler(orderSvc),
		Comment:      handler.NewCommentHandler(commentSvc),
		Evidence:     handler.NewEvidenceHandler(evidenceSvc),
		Notification: handler.NewNotificationHandler(notificationSvc),

		TenantUser:  handler.NewTenantUserHandler(userRepo),
		Dashboard:   handler.NewDashboardHandler(dashboardSvc),
		Manager:     handler.NewManagerHandler(managerSvc),
		Email:       handler.NewEmailHandler(emailSvc),
		Export:      handler.NewExportHandler(exportSvc),
		Search:      handler.NewSearchHandler(searchSvc),
		ActivityLog: handler.NewActivityLogHandler(activitySvc),

		PlatformTenant:      handler.NewPlatformTenantHandler(platformTenantSvc),
		PlatformUser:        handler.NewPlatformUserHandler(platformUserSvc),
		PlatformRight:       handler.NewPlatformRightHandler(rightRepo),
		PlatformSetting:     handler.NewPlatformSettingHandler(platformSettingSvc),
		PlatformOverview:    handler.NewPlatformOverviewHandler(reportingRepo),
		PlatformIntegration: handler.NewPlatformIntegrationHandler(pool),
		PlatformWebhook:     handler.NewPlatformWebhookHandler(webhookRepo),
		Impersonation:       handler.NewImpersonationHandler(impersonationSvc),
		PlatformEmail:       handler.NewPlatformEmailHandler(emailTemplateRepo, emailLogRepo),
		PlatformActivity:    handler.NewPlatformActivityHandler(activityRepo),
	}

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router.New(deps),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 20 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("api listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("bye")
}
