package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/adapter/generictemplate"
	"github.com/OpenNSW/core/artifact/adapter/workflowdef"
	"github.com/OpenNSW/core/artifact/loaders"
	"github.com/OpenNSW/core/authn"
	"github.com/OpenNSW/core/authz"
	"github.com/OpenNSW/core/cors"
	"github.com/OpenNSW/core/database"
	"github.com/OpenNSW/core/notification"
	"github.com/OpenNSW/core/notification/providers"
	"github.com/OpenNSW/core/payment"
	"github.com/OpenNSW/core/remote"
	"github.com/OpenNSW/core/storage"
	"github.com/OpenNSW/core/storage/drivers"
	"github.com/OpenNSW/core/taskflow/extensions"
	"github.com/OpenNSW/core/taskflow/orchestrator"
	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/renderer/zoneview"
	gormstore "github.com/OpenNSW/core/taskflow/store/gorm"
	"github.com/OpenNSW/core/temporal"
	"github.com/OpenNSW/core/trace"
	"github.com/OpenNSW/core/uiprojector"
	workflow "github.com/OpenNSW/core/workflow"
	"github.com/OpenNSW/nsw-srilanka/cmd/server/config"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cdn"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cusdec"
	"github.com/OpenNSW/nsw-srilanka/external-integration/payment/govpay"
	slpawebhook "github.com/OpenNSW/nsw-srilanka/external-integration/slpa/webhook"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	"github.com/OpenNSW/nsw-srilanka/internal/catalog"
	"github.com/OpenNSW/nsw-srilanka/internal/consignment"
	"github.com/OpenNSW/nsw-srilanka/internal/profile"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/cha"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/user"
	"github.com/OpenNSW/nsw-srilanka/internal/scopes"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/authzgate"
	taskauthzext "github.com/OpenNSW/nsw-srilanka/internal/tasks/extensions/authz"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/extensions/notify"
	taskplugins "github.com/OpenNSW/nsw-srilanka/internal/tasks/plugins"
	taskrenderer "github.com/OpenNSW/nsw-srilanka/internal/tasks/renderer"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
	"github.com/OpenNSW/nsw-srilanka/internal/trade"
	"github.com/OpenNSW/nsw-srilanka/internal/version"

	"github.com/LSFLK/argus/pkg/audit"

	"go.temporal.io/sdk/client"
	"gorm.io/gorm"
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
	Version             string   `json:"version"`
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
func Build(ctx context.Context, cfg *config.Config) (*App, error) { //nolint:gocyclo
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
	// The global catalog resolves the logical names used across configuration
	// (task-authz rule principals today) to this deployment's token roles and
	// client ids. Loaded once here and injected; no consumer reads the file.
	globalCatalog, err := catalog.Load(cfg.Server.CatalogConfigPath)
	if err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to load catalog: %w", err)
	}

	paymentRepo := payment.NewPaymentRepository(db)
	paymentRegistry, err := payment.NewRegistry(cfg.Server.PaymentMethodsConfigPath, map[string]payment.Factory{
		"govpay": govpay.NewGovPayGatewayFactory(govpay.NewRepositoryIdentityResolver(paymentRepo)),
	})
	if err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to load payment registry: %w", err)
	}
	paymentService := payment.NewPaymentService(paymentRepo, paymentRegistry)

	artifactLoader, err := loaders.New(ctx, cfg.ArtifactLoader)
	if err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to create artifact loader: %w", err)
	}

	artifactRegistry := artifact.NewRegistry(artifactLoader)

	manifestCfg, err := artifact.LoadManifest(ctx, artifactLoader)
	if err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to load artifact manifest: %w", err)
	}
	if err := artifact.RegisterFromConfig(artifactRegistry, manifestCfg); err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to register artifacts from manifest: %w", err)
	}

	chaService := cha.NewService(db)
	companyService := company.NewService(db)
	userProfileService := user.NewService(db)

	// Storage is built here rather than alongside its HTTP handler further
	// down: task plugins that attach uploaded files to an outbound call read
	// through this service, so it has to exist before the task stack (Stage 4).
	storageDriver, err := storage.NewStorageFromConfig(ctx, cfg.Storage)
	if err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}
	storageService := storage.NewService(storageDriver)

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
	var parentRunner workflow.TemporalManager
	onTaskCompleted := func(parentWorkflowID, parentRunID, parentNodeID string, finalVariables map[string]any) error {
		return parentRunner.TaskDone(context.Background(), parentWorkflowID, parentRunID, parentNodeID, finalVariables)
	}

	task, stopTask, err := initTask(db, temporalClient, paymentService, companyService, storageService, artifactRegistry, globalCatalog, cfg, onTaskCompleted)
	if err != nil {
		temporalClient.Close()
		_ = database.Close(db)
		return nil, err
	}
	tm := task.Manager
	paymentService.SetTaskCompleter(tm)

	// -------------------------------------------------------------------
	// Stage 5: Consignment Service & Workflow Parent Runner
	// -------------------------------------------------------------------
	auditClient := audit.NewClient(cfg.Audit)
	audit.InitializeGlobalAudit(auditClient)
	recorder := nswaudit.NewRecorder(auditClient)

	consignmentService, err := consignment.NewService(db, artifactRegistry, chaService, companyService, userProfileService, task.Store, globalCatalog.Roles)
	if err != nil {
		_ = stopTask()
		temporalClient.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to build consignment service: %w", err)
	}
	consignmentRouter, err := consignment.NewRouter(consignmentService, chaService, companyService, recorder, globalCatalog.Roles)
	if err != nil {
		_ = stopTask()
		temporalClient.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to build consignment router: %w", err)
	}

	pr, stopParentRunner, err := wireParentRunner(temporalClient, tm, consignmentService)
	if err != nil {
		_ = stopTask()
		temporalClient.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to wire parent runner: %w", err)
	}
	pr.RegisterDefinitionHandler(func(templateID string) (workflow.WorkflowDefinition, error) {
		def, err := workflowdef.Load(context.Background(), artifactRegistry, templateID)
		if err != nil {
			return workflow.WorkflowDefinition{}, fmt.Errorf("workflow definition %q not found in artifact registry: %w", templateID, err)
		}
		return def, nil
	})
	parentRunner = pr

	if err := consignmentService.RegisterWorkflowManager(parentRunner); err != nil {
		_ = stopParentRunner()
		_ = stopTask()
		temporalClient.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to register workflow manager with consignment service: %w", err)
	}

	// -------------------------------------------------------------------
	// Stage 6: Identity Provider (IDP) Authentication Manager
	// -------------------------------------------------------------------
	authnManager, err := authn.NewManager(userProfileService, cfg.Authn)
	if err != nil {
		_ = stopParentRunner()
		_ = stopTask()
		temporalClient.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to create authn manager: %w", err)
	}

	if err := authnManager.Health(); err != nil {
		_ = stopParentRunner()
		_ = stopTask()
		temporalClient.Close()
		_ = authnManager.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("authn system health check failed: %w", err)
	}

	// -------------------------------------------------------------------
	// Stage 7: HTTP Route & Middleware Registration
	// -------------------------------------------------------------------

	// ASYCUDA webhook stack.
	cdnRepo := cdn.NewDispatchNoteRepository(db)
	cdnWebhookService := cdn.NewCDNWebhookService(cdnRepo, db, tm)

	cusdecRepo := cusdec.NewDeclarationRepository(db)
	cusdecWebhookService := cusdec.NewWebhookService(cusdecRepo, db, tm)

	slceHandler := asycuda.NewHandler(cusdecWebhookService, cdnWebhookService)

	// SLPA webhook stack. SLPA signs its calls with a shared secret rather than
	// presenting an IdP token, so this handler owns its own authentication and
	// the route below carries no bearer middleware.
	// Refused at boot rather than started without the route. SLPA reports every
	// decision on this endpoint, so a deployment that cannot mount it accepts
	// service orders it can never hear the answer to: the callback 404s, their
	// retries stop, and the consignment waits on an approval that has already
	// happened. The outbound half of this integration fails the same way when
	// its own secret is missing (see the services registry), and a missing
	// secret is a deployment fault worth stopping for either way.
	slpaHandler, err := slpawebhook.NewHandler(
		slpawebhook.NewOrderEvents(db, tm),
		slpawebhook.NewInvoiceEvents(db, tm),
		cfg.Integrations.SLPAWebhook(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build the SLPA webhook handler: %w", err)
	}

	chaHandler := cha.NewHandler(chaService)
	companyHandler := company.NewHandler(companyService)
	profileHandler := profile.NewHandler(userProfileService, companyService)
	paymentHandler := payment.NewHTTPHandler(paymentService)
	// The storage driver and service behind this handler are built in Stage 2 —
	// task plugins that attach uploaded files to an outbound call read through
	// the service, so it has to exist before the task stack (Stage 4).
	storageHandler := storage.NewHTTPHandler(storageService)
	// The catalog is Layer 2 of task authorization on the read path: HandleGetTask
	// decides access from the role-tied ownership of the task's consignment.
	taskHandler := tasks.NewHTTPHandler(tm, task.Store, task.Assembler, taskCatalog(globalCatalog), cfg.Server.MaxRequestBytes)
	// Layer 1 of task authorization, shared by the read and write routes: attach
	// the caller's identity and a lazy ownership resolver for the PRE_RESUME authz
	// extension and the read evaluator to consume.
	taskAuthzGate, err := authzgate.NewMiddleware(ownershipResolver{svc: consignmentService}, companyIDResolver{svc: companyService}, globalCatalog.Roles)
	if err != nil {
		_ = stopParentRunner()
		_ = stopTask()
		temporalClient.Close()
		_ = authnManager.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to build task authz gate: %w", err)
	}

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
		_ = stopTask()
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
				Service:             "tnsw-api",
				Version:             version.Get(),
				UnhealthyComponents: unhealthy,
			})
			return
		}

		writeJSON(w, http.StatusOK, healthResponse{
			Status:  "ok",
			Service: "tnsw-api",
			Version: version.Get(),
		})
	})

	// API routes. Each handler is wrapped with authn (JWT validation) then a
	// scope gate. Order matters: withAuth injects the AuthContext; withScope
	// reads it. Public routes (local-dev storage) are below.
	mux.Handle("GET /api/v1/tasks/{id}", withAuth(withScope(scopes.TaskRead)(taskAuthzGate.Handler(http.HandlerFunc(taskHandler.HandleGetTask)))))
	mux.Handle("POST /api/v1/tasks/{id}/commands/{command}", withAuth(withScope(scopes.TaskWrite)(taskAuthzGate.Handler(http.HandlerFunc(taskHandler.HandleCompleteTaskStep)))))
	mux.Handle("POST /api/v1/tasks/{id}", withAuth(withScope(scopes.TaskWrite)(taskAuthzGate.Handler(http.HandlerFunc(taskHandler.HandleCompleteTaskStep)))))

	mux.Handle("GET /api/v1/chas", withAuth(withScope(scopes.CHARead)(http.HandlerFunc(chaHandler.HandleGetCHAs))))
	mux.Handle("GET /api/v1/companies", withAuth(withScope(scopes.CompanyRead)(http.HandlerFunc(companyHandler.HandleGetCompanies))))
	mux.Handle("GET /api/v1/users/me", withAuth(withScope(scopes.ProfileRead)(http.HandlerFunc(profileHandler.HandleGetProfile))))
	mux.Handle("POST /api/v1/consignments", withAuth(withScope(scopes.ConsignmentWrite)(http.HandlerFunc(consignmentRouter.HandleCreateConsignment))))
	mux.Handle("GET /api/v1/consignments/{id}/agency", withAuth(withScope(scopes.ConsignmentRead)(http.HandlerFunc(consignmentRouter.HandleGetConsignmentAgency))))
	mux.Handle("GET /api/v1/consignments/{id}", withAuth(withScope(scopes.ConsignmentRead)(http.HandlerFunc(consignmentRouter.HandleGetConsignmentByID))))
	mux.Handle("GET /api/v1/consignments", withAuth(withScope(scopes.ConsignmentRead)(http.HandlerFunc(consignmentRouter.HandleGetConsignments))))

	// Storage
	mux.Handle("POST /api/v1/storage", withAuth(withScope(scopes.StorageWrite)(http.HandlerFunc(storageHandler.Upload))))
	mux.Handle("GET /api/v1/storage/{key}", withAuth(withScope(scopes.StorageRead)(http.HandlerFunc(storageHandler.Download))))
	mux.Handle("DELETE /api/v1/storage/{key}", withAuth(withScope(scopes.StorageDelete)(http.HandlerFunc(storageHandler.Delete))))

	// Payment webhook endpoints. Requires valid JWT issued from nsw-srilanka's IDP with the appropriate scope. The gatewayId path param is used to resolve the correct payment gateway configuration for the webhook.
	// Authenticating the caller as the gateway itself is the gateway's own job:
	// core/payment calls PaymentGateway.VerifyWebhook before any reference lookup
	// or settlement, so each gateway checks the scheme it actually uses.
	mux.Handle("POST /api/v1/payments/{gatewayId}/webhook", withAuth(withScope(scopes.PaymentWebhooksProcess)(http.HandlerFunc(paymentHandler.HandleWebhook))))
	mux.Handle("POST /api/v1/payments/{gatewayId}/validate", withAuth(withScope(scopes.PaymentWebhooksValidate)(http.HandlerFunc(paymentHandler.HandleValidateReference))))

	// SLCE Webhook Endpoint (single central route handling all ASYCUDA/SLCE events).
	mux.Handle("POST /webhooks/slce", withAuth(withScope(scopes.SLCEWebhooksWrite)(http.HandlerFunc(slceHandler.HandleWebhook))))

	// SLPA Webhook Endpoint. Authenticated by the HMAC signature on the request
	// itself — see slpa.VerifySignature — so no token middleware here.
	mux.Handle("POST /webhooks/slpa", http.HandlerFunc(slpaHandler.HandleWebhook))

	// When using local storage, these endpoints serve as mocks for S3.
	if _, ok := storageDriver.(*drivers.LocalFSDriver); ok {
		mux.HandleFunc("PUT /api/v1/storage/{key}/content", storageHandler.UploadContentLocal)
		mux.HandleFunc("GET /api/v1/storage/{key}/content", storageHandler.DownloadContent)
	}

	// -------------------------------------------------------------------
	// Stage 8: Server Instantiation & Close Hook
	// -------------------------------------------------------------------
	handler := cors.CORS(&cfg.CORS)(trace.TraceMiddleware(mux))
	server := newHTTPServer(cfg.Server, handler)

	closeFn := func() error {
		var closeErrs []error

		if auditClient != nil && auditClient.IsEnabled() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := auditClient.Close(shutdownCtx); err != nil {
				closeErrs = append(closeErrs, fmt.Errorf("failed to close audit client: %w", err))
			}
			cancel()
		}

		if err := stopParentRunner(); err != nil {
			closeErrs = append(closeErrs, fmt.Errorf("failed to stop parent runner: %w", err))
		}
		if err := stopTask(); err != nil {
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

func newHTTPServer(cfg config.ServerConfig, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}
}

// parentWorkflowQueue is the Temporal task queue the macro/parent workflow
// runner polls.
const parentWorkflowQueue = "INTERPRETER_TASK_QUEUE"

// parentTaskActivator is the narrow surface wireParentRunner needs from the
// core orchestrator: when the parent graph reaches a Task node, StartTask
// spawns the corresponding task workflow. *orchestrator.TaskManager satisfies
// this directly.
type parentTaskActivator interface {
	StartTask(ctx context.Context, payload workflow.TaskPayload) (map[string]any, error)
}

// parentUpstreamService is the narrow surface wireParentRunner needs to notify
// a downstream domain (consignment) when a parent workflow completes.
// *consignment.Service satisfies this directly via its CompletionHandler method.
type parentUpstreamService interface {
	CompletionHandler(workflowID string, finalContext map[string]any) error
}

// wireParentRunner wires the core/workflow port of workflow.WireParentRunner.
// core ships no wrapper for this, so the wiring is inlined here, the only
// place that needs it.
//
// It starts the Temporal worker that runs macro/parent workflows on
// parentWorkflowQueue. When a parent workflow reaches a Task node, the
// activator's StartTask is invoked to spawn the corresponding task workflow.
// On parent-workflow completion, upstream.CompletionHandler is invoked so
// consignment can advance its own state.
func wireParentRunner(c client.Client, activator parentTaskActivator, upstream parentUpstreamService) (workflow.TemporalManager, func() error, error) {
	if activator == nil {
		return nil, nil, fmt.Errorf("parent task activator cannot be nil")
	}

	onActivation := func(payload workflow.TaskPayload) (map[string]any, error) {
		return activator.StartTask(context.Background(), payload)
	}

	onCompletion := func(workflowID string, finalVariables map[string]any) error {
		log.Printf("\n[Parent Workflow] Completed. Final state: %v\n", finalVariables)
		if upstream != nil {
			if err := upstream.CompletionHandler(workflowID, finalVariables); err != nil {
				return fmt.Errorf("upstream completion handler: %w", err)
			}
		}
		return nil
	}

	runner := workflow.NewTemporalManager(c, parentWorkflowQueue, onActivation, onCompletion)
	if err := runner.StartWorker(); err != nil {
		return nil, nil, fmt.Errorf("failed to start parent workflow worker: %w", err)
	}

	stop := func() error {
		runner.StopWorker()
		return nil
	}

	return runner, stop, nil
}

// taskStack bundles the core-orchestrator objects bootstrap needs to expose
// handlers and to wire the parent workflow runner. It is the local counterpart
// of the old taskv2.WireResult, now built directly against core/taskflow and
// core/workflow rather than through a country-agnostic wiring wrapper (core
// exposes orchestrator.NewTaskManager directly — see the integration guide).
type taskStack struct {
	Manager   *orchestrator.TaskManager
	Runner    workflow.TemporalManager
	Store     *gormstore.TaskStore
	Assembler *zoneview.ZoneViewAssembler
}

// ownershipResolver adapts the consignment service to
// authzgate.OwnershipResolver. A consignment that does not exist is reported as
// ("", "", nil) — the caller then owns neither side and is denied cleanly, the
// same fail-closed convention companyIDResolver follows, rather than surfacing a
// missing consignment as a 500.
//
// It wraps the same interface it produces: *consignment.Service already has the
// right shape, so all this adds is the error translation.
type ownershipResolver struct{ svc authzgate.OwnershipResolver }

func (r ownershipResolver) GetOwnership(ctx context.Context, consignmentID string) (string, string, error) {
	traderCompanyID, chaCompanyID, err := r.svc.GetOwnership(ctx, consignmentID)
	if err != nil {
		if errors.Is(err, consignment.ErrConsignmentNotFound) {
			return "", "", nil
		}
		return "", "", err
	}
	return traderCompanyID, chaCompanyID, nil
}

// taskCatalog narrows the global catalog to the slice task authorization needs.
// Both Layer-2 evaluators take the same value, so the mapping lives here rather
// than at each call site.
func taskCatalog(c *catalog.Catalog) taskauthz.Catalog {
	return taskauthz.Catalog{Roles: c.Roles, Clients: c.Clients}
}

// companyIDResolver adapts the company service to authzgate.CompanyResolver. A
// missing company profile is reported as ("", nil) so it denies cleanly rather
// than surfacing as a 500.
type companyIDResolver struct{ svc company.Service }

func (r companyIDResolver) CompanyIDByOUHandle(ctx context.Context, ouHandle string) (string, error) {
	rec, err := r.svc.GetCompanyByOUHandle(ctx, ouHandle)
	if err != nil {
		if errors.Is(err, company.ErrCompanyNotFound) {
			return "", nil
		}
		return "", err
	}
	if rec == nil {
		return "", nil
	}
	return rec.ID, nil
}

// registryTemplateProvider adapts the artifact registry to uiprojector's
// TemplateProvider contract. Generic templates (JSONForms schemas, markdown
// bodies, etc.) are resolved through generictemplate.Load.
type registryTemplateProvider struct {
	reg *artifact.Registry
}

func (p registryTemplateProvider) GetTemplate(ctx context.Context, id string) ([]byte, error) {
	raw, err := generictemplate.Load(ctx, p.reg, id)
	if err != nil {
		return nil, fmt.Errorf("template %q not found: %w", id, err)
	}
	return []byte(raw), nil
}

// initTask consolidates the task-orchestration engine registrations, remote
// configs, and wiring against core/taskflow + core/workflow. It builds and
// starts the orchestration stack on MICRO_WORKFLOW_QUEUE; the returned
// TemporalManager runs the per-task (micro) sub-workflows, while the
// parent/macro workflow runner is owned by the workflow package and wired
// separately (see Stage 5 below).
func initTask(
	db *gorm.DB,
	temporalClient client.Client,
	paymentService payment.PaymentService,
	companyService company.Service,
	storageService *storage.Service,
	artifactRegistry *artifact.Registry,
	globalCatalog *catalog.Catalog,
	cfg *config.Config,
	onTaskCompleted orchestrator.TaskCompletedCallback,
) (*taskStack, func() error, error) {
	// Initialize outbound HTTP caller configurations
	remoteManager := remote.NewManager()
	if err := remoteManager.LoadServices(cfg.Server.ServicesConfigPath); err != nil {
		return nil, nil, fmt.Errorf("failed to load remote services from %s: %w", cfg.Server.ServicesConfigPath, err)
	}

	// Instantiate flow plugins registry
	pluginsRegistry := plugins.NewRegistry()
	if err := taskplugins.Register(pluginsRegistry, remoteManager, paymentService, storageService, cfg.Server.ServiceURL, cfg.Server.Debug); err != nil {
		return nil, nil, fmt.Errorf("failed to register task plugins: %w", err)
	}
	if err := pluginsRegistry.Register("HSCODE_SPLIT_BUILDER", trade.NewGenericExecutorPlugin(trade.HscodeSplitBuilderFunc)); err != nil {
		return nil, nil, fmt.Errorf("failed to register HSCODE_SPLIT_BUILDER plugin: %w", err)
	}
	if err := pluginsRegistry.Register(taskplugins.TaskTypeCDNSplitBuilder, trade.NewGenericExecutorPlugin(taskplugins.CDNSplitBuilderFunc)); err != nil {
		return nil, nil, fmt.Errorf("failed to register %s plugin: %w", taskplugins.TaskTypeCDNSplitBuilder, err)
	}
	if err := pluginsRegistry.Register(taskplugins.TaskTypeSLPAGatePassSplitBuilder, trade.NewGenericExecutorPlugin(taskplugins.SLPAGatePassSplitBuilderFunc)); err != nil {
		return nil, nil, fmt.Errorf("failed to register %s plugin: %w", taskplugins.TaskTypeSLPAGatePassSplitBuilder, err)
	}
	if err := pluginsRegistry.Register(taskplugins.TaskTypeCDNResultsCollector, trade.NewGenericExecutorPlugin(taskplugins.CDNResultsCollectorFunc)); err != nil {
		return nil, nil, fmt.Errorf("failed to register %s plugin: %w", taskplugins.TaskTypeCDNResultsCollector, err)
	}
	if err := pluginsRegistry.Register("CHA_PERSIST_WRITER", trade.NewCHAPersistPlugin(db, companyService)); err != nil {
		return nil, nil, fmt.Errorf("failed to register CHA_PERSIST_WRITER plugin: %w", err)
	}

	taskStore := gormstore.New(db)

	// Construct UI projectors and the renderer/zoneview assembler
	projectors := append(uiprojector.DefaultProjectors(), taskrenderer.NewPaymentProjector())
	uiAssembler, err := uiprojector.NewAssembler(registryTemplateProvider{reg: artifactRegistry}, projectors)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build ui assembler: %w", err)
	}
	taskRenderer := zoneview.NewTaskRenderer(uiAssembler)
	zoneAssembler := zoneview.NewZoneViewAssembler(taskRenderer)

	var tm *orchestrator.TaskManager

	// Handlers for events on the per-task (micro) sub-workflows running on
	// MICRO_WORKFLOW_QUEUE. Nodes inside a task workflow activate subtasks
	// via tm.StartSubTask, which dispatches to the matching plugin.
	microActivationHandler := func(payload workflow.TaskPayload) (map[string]any, error) {
		log.Printf("\n[Micro Workflow] SubTask activated: node=%s template=%s\n", payload.NodeID, payload.TaskTemplateID)
		if tm == nil {
			return nil, fmt.Errorf("task manager is not initialized (misconfiguration)")
		}
		return tm.StartSubTask(context.Background(), payload)
	}

	microCompletionHandler := func(workflowID string, finalVariables map[string]any) error {
		log.Printf("\n[Micro Workflow] Completed. Final state: %v\n", finalVariables)
		if tm == nil {
			return fmt.Errorf("task manager is not initialized (misconfiguration)")
		}
		return tm.HandleTaskCompletion(context.Background(), workflowID, finalVariables)
	}

	workflowRunner := workflow.NewTemporalManager(temporalClient, "MICRO_WORKFLOW_QUEUE", microActivationHandler, microCompletionHandler)

	notifManager, err := notification.NewManager(cfg.Notification,
		providers.NewEmailProvider(), providers.NewSMSProvider())
	if err != nil {
		return nil, nil, fmt.Errorf("notification manager: %w", err)
	}

	extensionsRegistry := extensions.NewRegistry()
	if err := notify.Register(extensionsRegistry, notifManager, registryTemplateProvider{reg: artifactRegistry}, cfg.Server.Debug); err != nil {
		return nil, nil, fmt.Errorf("register notification extension: %w", err)
	}
	if err := taskauthzext.Register(extensionsRegistry, taskCatalog(globalCatalog)); err != nil {
		return nil, nil, fmt.Errorf("register authz extension: %w", err)
	}
	tm = orchestrator.NewTaskManager(taskStore, artifactRegistry, pluginsRegistry, extensionsRegistry, workflowRunner, onTaskCompleted, taskRenderer)

	if err := workflowRunner.StartWorker(); err != nil {
		return nil, nil, fmt.Errorf("failed to start micro workflow worker: %w", err)
	}

	stop := func() error {
		workflowRunner.StopWorker()
		return nil
	}

	return &taskStack{
		Manager:   tm,
		Runner:    workflowRunner,
		Store:     taskStore,
		Assembler: zoneAssembler,
	}, stop, nil
}
