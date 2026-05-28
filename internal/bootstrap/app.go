package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	engine "github.com/OpenNSW/go-temporal-workflow"
	flowplugins "github.com/OpenNSW/nsw-task-flow/plugins"
	"github.com/OpenNSW/nsw/internal/workflow"

	"github.com/OpenNSW/nsw/internal/auth"
	"github.com/OpenNSW/nsw/internal/config"
	"github.com/OpenNSW/nsw/internal/consignment"
	"github.com/OpenNSW/nsw/internal/database"
	"github.com/OpenNSW/nsw/internal/hscode"
	"github.com/OpenNSW/nsw/internal/middleware"
	"github.com/OpenNSW/nsw/internal/payments"
	"github.com/OpenNSW/nsw/internal/profile/cha"
	"github.com/OpenNSW/nsw/internal/profile/company"
	"github.com/OpenNSW/nsw/internal/profile/user"
	"github.com/OpenNSW/nsw/internal/taskv2"
	taskv2plugins "github.com/OpenNSW/nsw/internal/taskv2/plugins"
	"github.com/OpenNSW/nsw/internal/temporal"
	"github.com/OpenNSW/nsw/internal/workflow/service"
	"github.com/OpenNSW/nsw/pkg/remote"
	"github.com/OpenNSW/nsw/pkg/storage"
	"github.com/OpenNSW/nsw/pkg/storage/drivers"

	"github.com/OpenNSW/nsw/pkg/notification"
	"github.com/OpenNSW/nsw/pkg/notification/channels"
)

// App contains an initialized HTTP server and cleanup hooks.
type App struct {
	Server              *http.Server
	NotificationManager *notification.Manager
	close               func() error
}

// Close releases resources initialized during bootstrap.
func (a *App) Close() error {
	if a == nil || a.close == nil {
		return nil
	}
	return a.close()
}

// healthResponse is the JSON shape returned by the health endpoint in all cases.
// UnhealthyComponents is omitted on success and populated with the names of all
// failing subsystems on failure.
type healthResponse struct {
	Status              string   `json:"status"`
	Service             string   `json:"service"`
	UnhealthyComponents []string `json:"unhealthy_components,omitempty"`
}

// writeJSON sets the Content-Type header, writes the status code, and encodes v as JSON.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Build initializes dependencies and returns a fully wired application server.
func Build(ctx context.Context, cfg *config.Config) (*App, error) {
	db, err := database.New(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := database.HealthCheck(db); err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("database health check failed: %w", err)
	}

	paymentRepo := payments.NewPaymentRepository(db)
	paymentService, err := payments.NewPaymentService(paymentRepo, cfg.Server.PaymentMethodsConfigPath)
	if err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to initialize payment service: %w", err)
	}

	templateService := service.NewTemplateService(db)
	chaService := cha.NewService(db)
	companyService := company.NewService(db)
	userProfileService := user.NewService(db)
	hsCodeService := hscode.NewService(db)

	temporalClient, err := temporal.NewClient(cfg.Temporal)
	if err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to create temporal client: %w", err)
	}

	// parentRunner is forward-declared so the taskv2 completion callback can
	// close over it. It is assigned below after WireParentRunner returns; the
	// closure is only invoked when a task workflow finishes, by which point
	// the assignment has already happened.
	var parentRunner engine.TemporalManager
	onTaskCompleted := func(parentWorkflowID, parentRunID, parentNodeID string, finalVariables map[string]any) error {
		return parentRunner.TaskDone(context.Background(), parentWorkflowID, parentRunID, parentNodeID, finalVariables)
	}

	remoteManager := remote.NewManager()
	if err := remoteManager.LoadServices(cfg.Server.ServicesConfigPath); err != nil {
		temporalClient.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to load remote services from %s: %w", cfg.Server.ServicesConfigPath, err)
	}

	pluginsRegistry := flowplugins.NewRegistry()
	if err := taskv2plugins.Register(pluginsRegistry, remoteManager, paymentService, cfg.Server.ServiceURL, cfg.Server.Debug); err != nil {
		temporalClient.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to register taskv2 plugins: %w", err)
	}

	taskV2, stopTaskV2, err := taskv2.WireTaskV2(db, temporalClient, pluginsRegistry, paymentService, onTaskCompleted)
	if err != nil {
		temporalClient.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to wire taskv2: %w", err)
	}
	tm := taskV2.Manager

	consignmentService := consignment.NewService(db, templateService, chaService, companyService, userProfileService, hsCodeService)
	consignmentRouter := consignment.NewRouter(consignmentService, chaService, companyService)

	pr, stopParentRunner, err := workflow.WireParentRunner(temporalClient, tm, consignmentService)
	if err != nil {
		_ = stopTaskV2()
		temporalClient.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to wire parent runner: %w", err)
	}
	parentRunner = pr

	if err := consignmentService.RegisterWorkflowManager(parentRunner); err != nil {
		_ = stopParentRunner()
		_ = stopTaskV2()
		temporalClient.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to register workflow manager with consignment service: %w", err)
	}

	hsCodeRouter := hscode.NewRouter(hsCodeService)
	chaHandler := cha.NewHandler(chaService)
	companyHandler := company.NewHandler(companyService)

	storageDriver, err := storage.NewStorageFromConfig(ctx, cfg.Storage)
	if err != nil {
		_ = stopParentRunner()
		_ = stopTaskV2()
		temporalClient.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}
	storageService := storage.NewService(storageDriver)
	storageHandler := storage.NewHTTPHandler(storageService)

	paymentHandler := payments.NewHTTPHandler(paymentService)

	authManager, err := auth.NewManager(userProfileService, cfg.Auth)
	if err != nil {
		_ = stopParentRunner()
		_ = stopTaskV2()
		temporalClient.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to create auth manager: %w", err)
	}

	if err := authManager.Health(); err != nil {
		_ = stopParentRunner()
		_ = stopTaskV2()
		temporalClient.Close()
		_ = authManager.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("auth system health check failed: %w", err)
	}

	// Initialize notification manager
	notificationManager := notification.NewManager()
	emailChannel := channels.NewEmailChannel(notification.EmailConfig{
		SMTPHost:     cfg.Notification.SMTPHost,
		SMTPPort:     cfg.Notification.SMTPPort,
		SMTPUsername: cfg.Notification.SMTPUsername,
		SMTPPassword: cfg.Notification.SMTPPassword,
		SMTPSender:   cfg.Notification.SMTPSender,
		TemplateRoot: cfg.Notification.TemplateRoot,
	})
	notificationManager.RegisterEmailChannel(emailChannel)

	// TODO: Add SMS channel if needed
	// smsChannel := channels.NewSMSChannel(...)
	// notificationManager.RegisterSMSChannel(smsChannel)

	taskV2Handler := taskv2.NewHTTPHandler(tm, taskV2.Store, taskV2.Assembler)

	// withAuth wraps an individual handler with the authentication middleware.
	withAuth := authManager.Middleware()

	mux := http.NewServeMux()

	// Health check is public and returns JSON in all cases.
	// On failure, the component field identifies which subsystem is unhealthy
	// without exposing internal error details.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		var unhealthy []string

		if err := database.HealthCheck(db); err != nil {
			unhealthy = append(unhealthy, "database")
		}
		if err := authManager.Health(); err != nil {
			unhealthy = append(unhealthy, "auth")
		}

		if len(unhealthy) > 0 {
			writeJSON(w, http.StatusServiceUnavailable, healthResponse{
				Status:              "error",
				Service:             "nsw-backend",
				UnhealthyComponents: unhealthy,
			})
			return
		}

		writeJSON(w, http.StatusOK, healthResponse{
			Status:  "ok",
			Service: "nsw-backend",
		})
	})

	// API routes. Each handler is individually wrapped with auth,
	// so public or differently-authenticated routes can be added
	// alongside these without restructuring the mux.
	mux.Handle("GET /api/v1/tasks/{id}", withAuth(http.HandlerFunc(taskV2Handler.HandleGetTask)))
	mux.Handle("POST /api/v1/tasks/{id}", withAuth(http.HandlerFunc(taskV2Handler.HandleCompleteTaskStep)))
	// TODO(oga-callback): remove once OGA POSTs directly to /api/v1/tasks/{id}
	// with the bare reviewer payload. This legacy route accepts OGA's
	// {task_id, workflow_id, payload:{action, content}} envelope and the
	// handler unwraps payload.content + falls back to body-level task_id.
	mux.Handle("POST /api/v1/tasks", withAuth(http.HandlerFunc(taskV2Handler.HandleCompleteTaskStep)))
	mux.Handle("GET /api/v1/hscodes", withAuth(http.HandlerFunc(hsCodeRouter.HandleGetAll)))
	mux.Handle("GET /api/v1/chas", withAuth(http.HandlerFunc(chaHandler.HandleGetCHAs)))
	mux.Handle("GET /api/v1/companies", withAuth(http.HandlerFunc(companyHandler.HandleGetCompanies)))
	mux.Handle("POST /api/v1/consignments", withAuth(http.HandlerFunc(consignmentRouter.HandleCreateConsignment)))
	mux.Handle("GET /api/v1/consignments/{id}", withAuth(http.HandlerFunc(consignmentRouter.HandleGetConsignmentByID)))
	mux.Handle("PUT /api/v1/consignments/{id}", withAuth(http.HandlerFunc(consignmentRouter.HandleInitializeConsignment)))
	mux.Handle("GET /api/v1/consignments", withAuth(http.HandlerFunc(consignmentRouter.HandleGetConsignments)))

	// Storage
	mux.Handle("POST /api/v1/storage", withAuth(http.HandlerFunc(storageHandler.Upload)))
	mux.Handle("GET /api/v1/storage/{key}", withAuth(http.HandlerFunc(storageHandler.Download)))
	mux.Handle("DELETE /api/v1/storage/{key}", withAuth(http.HandlerFunc(storageHandler.Delete)))

	// External Webhooks bypass standard JWT auth.
	// They should use webhook signatures, implemented in the handler directly or via specialized middleware.
	mux.Handle("POST /api/v1/payments/webhook", http.HandlerFunc(paymentHandler.HandleWebhook))
	mux.Handle("POST /api/v1/payments/validate", http.HandlerFunc(paymentHandler.HandleValidateReference))

	// When using local storage, these endpoints serve as mocks for S3.
	if _, ok := storageDriver.(*drivers.LocalFSDriver); ok {
		mux.HandleFunc("PUT /api/v1/storage/{key}/content", storageHandler.UploadContentLocal)
		mux.HandleFunc("GET /api/v1/storage/{key}/content", storageHandler.DownloadContent)
	}

	handler := middleware.CORS(&cfg.CORS)(mux)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: handler,
	}

	closeFn := func() error {
		var closeErrs []error

		if err := stopParentRunner(); err != nil {
			closeErrs = append(closeErrs, fmt.Errorf("failed to stop parent runner: %w", err))
		}
		if err := stopTaskV2(); err != nil {
			closeErrs = append(closeErrs, fmt.Errorf("failed to stop taskv2: %w", err))
		}
		temporalClient.Close()
		if err := authManager.Close(); err != nil {
			closeErrs = append(closeErrs, fmt.Errorf("failed to close auth manager: %w", err))
		}
		if err := database.Close(db); err != nil {
			closeErrs = append(closeErrs, fmt.Errorf("failed to close database: %w", err))
		}

		return errors.Join(closeErrs...)
	}

	return &App{
		Server:              server,
		NotificationManager: notificationManager,
		close:               closeFn,
	}, nil
}
