package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	engine "github.com/OpenNSW/go-temporal-workflow"
	flowplugins "github.com/OpenNSW/nsw-task-flow/plugins"
	"github.com/OpenNSW/nsw/backend/internal/workflow"
	"github.com/OpenNSW/nsw/backend/srilanka/internal/scopes"
	"go.temporal.io/sdk/client"
	"gorm.io/gorm"

	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/core/authz"
	"github.com/OpenNSW/core/cors"
	"github.com/OpenNSW/core/database"
	"github.com/OpenNSW/core/storage"
	"github.com/OpenNSW/core/temporal"
	"github.com/OpenNSW/nsw/backend/internal/consignment"
	"github.com/OpenNSW/nsw/backend/internal/hscode"
	"github.com/OpenNSW/nsw/backend/internal/payments"
	"github.com/OpenNSW/nsw/backend/internal/profile/cha"
	"github.com/OpenNSW/nsw/backend/internal/profile/company"
	"github.com/OpenNSW/nsw/backend/internal/profile/user"
	"github.com/OpenNSW/nsw/backend/internal/taskv2"
	taskv2plugins "github.com/OpenNSW/nsw/backend/internal/taskv2/plugins"
	"github.com/OpenNSW/nsw/backend/internal/taskv2/registry"
	taskrenderer "github.com/OpenNSW/nsw/backend/internal/taskv2/renderer"
	"github.com/OpenNSW/nsw/backend/internal/workflow/service"
	"github.com/OpenNSW/nsw/backend/pkg/remote"
	"github.com/OpenNSW/nsw/backend/pkg/storage/drivers"
	"github.com/OpenNSW/nsw/backend/pkg/uiprojector"
	"github.com/OpenNSW/nsw/backend/srilanka/cmd/server/config"
	"github.com/OpenNSW/nsw/backend/srilanka/internal/trade"
)

// App contains an initialized HTTP server and cleanup hooks.
type App struct {
	Server *http.Server
	close  func() error
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
// The initialization flow is structured in distinct stages to ensure readability.
func Build(ctx context.Context, cfg *config.Config) (*App, error) {
	// -------------------------------------------------------------------
	// Stage 1: Relational Database & Connection Health Check
	// -------------------------------------------------------------------
	db, err := database.New(cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := database.HealthCheck(db); err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("database health check failed: %w", err)
	}

	// -------------------------------------------------------------------
	// Stage 2: Domain Core Repositories & Base Services
	// -------------------------------------------------------------------
	paymentRepo := payments.NewPaymentRepository(db)
	paymentService, err := payments.NewPaymentService(paymentRepo, cfg.Server.PaymentMethodsConfigPath)
	if err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to initialize payment service: %w", err)
	}

	templateRegistry := registry.NewInMemRegistry()
	// TODO: remove the hardcoded load commands and move to runtime params.
	if err := registry.LoadConfigsInto(templateRegistry, "configs/fcau"); err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to load taskv2 configs: %w", err)
	}
	if err := registry.LoadConfigsInto(templateRegistry, "configs/npqs"); err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to load taskv2 configs for npqs: %w", err)
	}
	if err := registry.LoadConfigsInto(templateRegistry, "configs/trade"); err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to load taskv2 configs for trade: %w", err)
	}

	templateService := service.NewTemplateService(db).WithRegistry(templateRegistry)
	chaService := cha.NewService(db)
	companyService := company.NewService(db)
	userProfileService := user.NewService(db)
	hsCodeService := hscode.NewService(db)

	// -------------------------------------------------------------------
	// Stage 3: Temporal Orchestration Engine Client
	// -------------------------------------------------------------------
	temporalClient, err := temporal.NewClient(cfg.Temporal)
	if err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to create temporal client: %w", err)
	}

	// -------------------------------------------------------------------
	// Stage 4: Task V2 Sub-System Setup
	// -------------------------------------------------------------------
	// parentRunner is forward-declared so the taskv2 completion callback can
	// close over it. It is assigned below after WireParentRunner returns; the
	// closure is only invoked when a task workflow finishes, by which point
	// the assignment has already happened.
	var parentRunner engine.TemporalManager
	onTaskCompleted := func(parentWorkflowID, parentRunID, parentNodeID string, finalVariables map[string]any) error {
		return parentRunner.TaskDone(context.Background(), parentWorkflowID, parentRunID, parentNodeID, finalVariables)
	}

	taskV2, stopTaskV2, err := initTaskV2(db, temporalClient, paymentService, companyService, templateRegistry, cfg, onTaskCompleted)
	if err != nil {
		temporalClient.Close()
		_ = database.Close(db)
		return nil, err
	}
	tm := taskV2.Manager
	paymentService.SetTaskCompleter(tm)

	// -------------------------------------------------------------------
	// Stage 5: Consignment Service & Workflow Parent Runner
	// -------------------------------------------------------------------
	consignmentService := consignment.NewService(db, templateService, chaService, companyService, userProfileService, hsCodeService, taskV2.Store)
	consignmentRouter := consignment.NewRouter(consignmentService, chaService, companyService)

	pr, stopParentRunner, err := workflow.WireParentRunner(temporalClient, tm, consignmentService)
	if err != nil {
		_ = stopTaskV2()
		temporalClient.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to wire parent runner: %w", err)
	}
	pr.RegisterDefinitionHandler(func(templateID string) (engine.WorkflowDefinition, error) {
		def, ok := templateRegistry.GetWorkflow(templateID)
		if !ok {
			return engine.WorkflowDefinition{}, fmt.Errorf("workflow definition %q not found in registry", templateID)
		}
		return def, nil
	})
	parentRunner = pr

	if err := consignmentService.RegisterWorkflowManager(parentRunner); err != nil {
		_ = stopParentRunner()
		_ = stopTaskV2()
		temporalClient.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to register workflow manager with consignment service: %w", err)
	}

	// -------------------------------------------------------------------
	// Stage 6: File Storage Provider Setup
	// -------------------------------------------------------------------
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

	// -------------------------------------------------------------------
	// Stage 7: Identity Provider (IDP) Authentication Manager
	// -------------------------------------------------------------------
	authnManager, err := authn.NewManager(userProfileService, cfg.Authn)
	if err != nil {
		_ = stopParentRunner()
		_ = stopTaskV2()
		temporalClient.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to create authn manager: %w", err)
	}

	if err := authnManager.Health(); err != nil {
		_ = stopParentRunner()
		_ = stopTaskV2()
		temporalClient.Close()
		_ = authnManager.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("authn system health check failed: %w", err)
	}

	// -------------------------------------------------------------------
	// Stage 8: HTTP Route & Middleware Registration
	// -------------------------------------------------------------------
	hsCodeRouter := hscode.NewRouter(hsCodeService)
	chaHandler := cha.NewHandler(chaService)
	companyHandler := company.NewHandler(companyService)
	paymentHandler := payments.NewHTTPHandler(paymentService)
	taskV2Handler := taskv2.NewHTTPHandler(tm, taskV2.Store, taskV2.Assembler)

	// withAuth wraps an individual handler with the authentication middleware.
	withAuth := authnManager.RequireAuthMiddleware()

	// authzr gates routes by the OAuth2 scopes carried on the token.
	// The extractor bridges the authn layer (authn.GetAuthContext) into the
	// generic authz.Principal interface — authz imports nothing from core/authn.
	authzr, err := authz.New(func(ctx context.Context) (authz.Principal, bool) {
		ac := authn.GetAuthContext(ctx)
		if ac == nil || ac.Type() == "" {
			return nil, false
		}
		return ac, true
	})
	if err != nil {
		_ = stopParentRunner()
		_ = stopTaskV2()
		temporalClient.Close()
		_ = authnManager.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to create authz: %w", err)
	}
	// withScope returns a middleware requiring the given scope; compose after withAuth
	// so the authn context is already injected when the scope check runs.
	withScope := func(scope string) func(http.Handler) http.Handler {
		return authzr.RequireScope(scope)
	}

	mux := http.NewServeMux()

	// Health check is public and returns JSON in all cases.
	// On failure, the component field identifies which subsystem is unhealthy
	// without exposing internal error details.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		var unhealthy []string

		if err := database.HealthCheck(db); err != nil {
			unhealthy = append(unhealthy, "database")
		}
		if err := authnManager.Health(); err != nil {
			unhealthy = append(unhealthy, "authn")
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

	// API routes. Each handler is wrapped with authn (JWT validation) then a
	// scope gate. Order matters: withAuth injects the AuthContext; withScope
	// reads it. Public routes (payments, local-dev storage) are below.
	mux.Handle("GET /api/v1/tasks/{id}", withAuth(withScope(scopes.TaskRead)(http.HandlerFunc(taskV2Handler.HandleGetTask))))
	mux.Handle("POST /api/v1/tasks/{id}", withAuth(withScope(scopes.TaskWrite)(http.HandlerFunc(taskV2Handler.HandleCompleteTaskStep))))

	// TODO(oga-callback): remove once OGA POSTs directly to /api/v1/tasks/{id}
	// with the bare reviewer payload. This legacy route accepts OGA's
	// {task_id, workflow_id, payload:{action, content}} envelope and the
	// handler unwraps payload.content + falls back to body-level task_id.
	mux.Handle("POST /api/v1/tasks", withAuth(withScope(scopes.TaskWrite)(http.HandlerFunc(taskV2Handler.HandleCompleteTaskStep))))

	mux.Handle("GET /api/v1/hscodes", withAuth(withScope(scopes.HSCodeRead)(http.HandlerFunc(hsCodeRouter.HandleGetAll))))
	mux.Handle("GET /api/v1/chas", withAuth(withScope(scopes.CHARead)(http.HandlerFunc(chaHandler.HandleGetCHAs))))
	mux.Handle("GET /api/v1/companies", withAuth(withScope(scopes.CompanyRead)(http.HandlerFunc(companyHandler.HandleGetCompanies))))
	mux.Handle("POST /api/v1/consignments", withAuth(withScope(scopes.ConsignmentWrite)(http.HandlerFunc(consignmentRouter.HandleCreateConsignment))))
	mux.Handle("POST /api/v1/consignments/start", withAuth(withScope(scopes.ConsignmentWrite)(http.HandlerFunc(consignmentRouter.HandleStartConsignment))))
	mux.Handle("GET /api/v1/consignments/{id}", withAuth(withScope(scopes.ConsignmentRead)(http.HandlerFunc(consignmentRouter.HandleGetConsignmentByID))))
	mux.Handle("PUT /api/v1/consignments/{id}", withAuth(withScope(scopes.ConsignmentWrite)(http.HandlerFunc(consignmentRouter.HandleInitializeConsignment))))
	mux.Handle("GET /api/v1/consignments", withAuth(withScope(scopes.ConsignmentRead)(http.HandlerFunc(consignmentRouter.HandleGetConsignments))))

	// Storage
	mux.Handle("POST /api/v1/storage", withAuth(withScope(scopes.StorageWrite)(http.HandlerFunc(storageHandler.Upload))))
	mux.Handle("GET /api/v1/storage/{key}", withAuth(withScope(scopes.StorageRead)(http.HandlerFunc(storageHandler.Download))))
	mux.Handle("DELETE /api/v1/storage/{key}", withAuth(withScope(scopes.StorageDelete)(http.HandlerFunc(storageHandler.Delete))))

	// External Webhooks bypass standard JWT authn.
	// They should use webhook signatures, implemented in the handler directly or via specialized middleware.
	mux.Handle("POST /api/v1/payments/webhook", http.HandlerFunc(paymentHandler.HandleWebhook))
	mux.Handle("POST /api/v1/payments/validate", http.HandlerFunc(paymentHandler.HandleValidateReference))

	// When using local storage, these endpoints serve as mocks for S3.
	if _, ok := storageDriver.(*drivers.LocalFSDriver); ok {
		mux.HandleFunc("PUT /api/v1/storage/{key}/content", storageHandler.UploadContentLocal)
		mux.HandleFunc("GET /api/v1/storage/{key}/content", storageHandler.DownloadContent)
	}

	// -------------------------------------------------------------------
	// Stage 9: Server Instantiation & Close Hook
	// -------------------------------------------------------------------
	handler := cors.CORS(&cfg.CORS)(mux)

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
		if err := authnManager.Close(); err != nil {
			closeErrs = append(closeErrs, fmt.Errorf("failed to close authn manager: %w", err))
		}
		if err := database.Close(db); err != nil {
			closeErrs = append(closeErrs, fmt.Errorf("failed to close database: %w", err))
		}

		return errors.Join(closeErrs...)
	}

	return &App{
		Server: server,
		close:  closeFn,
	}, nil
}

// initTaskV2 consolidates Task V2 engine registrations, remote configs, and wiring.
func initTaskV2(
	db *gorm.DB,
	temporalClient client.Client,
	paymentService payments.PaymentService,
	companyService company.Service,
	templateRegistry *registry.InMemRegistry,
	cfg *config.Config,
	onTaskCompleted func(string, string, string, map[string]any) error,
) (*taskv2.WireResult, func() error, error) {
	// Initialize outbound HTTP caller configurations
	remoteManager := remote.NewManager()
	if err := remoteManager.LoadServices(cfg.Server.ServicesConfigPath); err != nil {
		return nil, nil, fmt.Errorf("failed to load remote services from %s: %w", cfg.Server.ServicesConfigPath, err)
	}

	// Instantiate flow plugins registry
	pluginsRegistry := flowplugins.NewRegistry()
	if err := taskv2plugins.Register(pluginsRegistry, remoteManager, paymentService, cfg.Server.ServiceURL, cfg.Server.Debug); err != nil {
		return nil, nil, fmt.Errorf("failed to register taskv2 plugins: %w", err)
	}
	if err := pluginsRegistry.Register("HSCODE_SPLIT_BUILDER", trade.NewGenericExecutorPlugin(trade.HscodeSplitBuilderFunc)); err != nil {
		return nil, nil, fmt.Errorf("failed to register HSCODE_SPLIT_BUILDER plugin: %w", err)
	}
	if err := pluginsRegistry.Register("CHA_PERSIST_WRITER", trade.NewCHAPersistPlugin(db, companyService)); err != nil {
		return nil, nil, fmt.Errorf("failed to register CHA_PERSIST_WRITER plugin: %w", err)
	}

	// Construct UI projectors
	projectors := append(uiprojector.DefaultProjectors(), taskrenderer.NewPaymentProjector(paymentService))

	// Wire Task V2 orchestration nodes and workers
	taskV2, stopTaskV2, err := taskv2.WireTaskV2(db, temporalClient, pluginsRegistry, templateRegistry, projectors, onTaskCompleted)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to wire taskv2: %w", err)
	}

	return taskV2, stopTaskV2, nil
}
