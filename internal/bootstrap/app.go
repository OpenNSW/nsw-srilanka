package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/adapter/generictemplate"
	"github.com/OpenNSW/core/artifact/adapter/steptemplate"
	"github.com/OpenNSW/core/artifact/adapter/workflowdef"
	"github.com/OpenNSW/core/artifact/loaders"
	"github.com/OpenNSW/core/authz"
	"github.com/OpenNSW/core/cors"
	notification "github.com/OpenNSW/core/notifications"
	"github.com/OpenNSW/core/notifications/providers"
	"github.com/OpenNSW/core/payment"
	"github.com/OpenNSW/core/refid"
	"github.com/OpenNSW/core/remote"
	sharedaudit "github.com/OpenNSW/core/shared/audit"
	"github.com/OpenNSW/core/taskflow/extensions"
	"github.com/OpenNSW/core/taskflow/orchestrator"
	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/renderconfig"
	"github.com/OpenNSW/core/taskflow/renderer/zoneview"
	"github.com/OpenNSW/core/taskflow/store"
	gormstore "github.com/OpenNSW/core/taskflow/store/gorm"
	"github.com/OpenNSW/core/temporal"
	"github.com/OpenNSW/core/trace"
	"github.com/OpenNSW/core/uiprojector"
	workflow "github.com/OpenNSW/core/workflow"
	"github.com/OpenNSW/nsw-srilanka/cmd/server/config"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cdn"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cusdec"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/parkedtask"
	"github.com/OpenNSW/nsw-srilanka/external-integration/payment/govpay"
	slpawebhook "github.com/OpenNSW/nsw-srilanka/external-integration/slpa/webhook"
	nswaudit "github.com/OpenNSW/nsw-srilanka/internal/audit"
	nswauthn "github.com/OpenNSW/nsw-srilanka/internal/authn"
	"github.com/OpenNSW/nsw-srilanka/internal/caseflow"
	"github.com/OpenNSW/nsw-srilanka/internal/caseparty"
	"github.com/OpenNSW/nsw-srilanka/internal/catalog"
	"github.com/OpenNSW/nsw-srilanka/internal/consignment"
	"github.com/OpenNSW/nsw-srilanka/internal/database"
	"github.com/OpenNSW/nsw-srilanka/internal/htmltemplate"
	nswpayment "github.com/OpenNSW/nsw-srilanka/internal/payment"
	"github.com/OpenNSW/nsw-srilanka/internal/profile"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/cha"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/user"
	"github.com/OpenNSW/nsw-srilanka/internal/scopes"
	"github.com/OpenNSW/nsw-srilanka/internal/securitylog"
	"github.com/OpenNSW/nsw-srilanka/internal/staticdata"
	nswstorage "github.com/OpenNSW/nsw-srilanka/internal/storage"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileaccess"
	"github.com/OpenNSW/nsw-srilanka/internal/storage/filetoken"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/authzgate"
	taskauthzext "github.com/OpenNSW/nsw-srilanka/internal/tasks/extensions/authz"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/extensions/notify"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/extensions/payloadschema"
	taskplugins "github.com/OpenNSW/nsw-srilanka/internal/tasks/plugins"
	taskrenderer "github.com/OpenNSW/nsw-srilanka/internal/tasks/renderer"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/systemcaller"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/taskauthz"
	"github.com/OpenNSW/nsw-srilanka/internal/telemetry"
	"github.com/OpenNSW/nsw-srilanka/internal/trade"
	"github.com/OpenNSW/nsw-srilanka/internal/version"

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
func Build(ctx context.Context, cfg *config.Config) (app *App, err error) { //nolint:gocyclo
	// -------------------------------------------------------------------
	// Stage 1: Relational Database & Connection Health Check
	// -------------------------------------------------------------------
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := database.HealthCheck(ctx, db); err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("database health check failed: %w", err)
	}

	// OTLP before the auditor and workers so in-flight workflow audit on
	// restart is not dropped while telemetry was still starting. On a later
	// Build failure, shut telemetry down here; App.Close does it on success.
	tel, err := telemetry.Start(ctx, cfg.Otel)
	if err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to start telemetry: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tel.Shutdown(context.Background())
		}
	}()
	exportLogs := tel.SlogHandler()
	configureLogging(cfg, os.Stdout, exportLogs)
	var auditLogger *slog.Logger
	if h := auditExportHandler(cfg, exportLogs); h != nil {
		auditLogger = slog.New(h)
	}
	logAuditExportStatus(cfg, exportLogs != nil)

	// -------------------------------------------------------------------
	// Stage 2: Domain Core Repositories & Base Services
	// -------------------------------------------------------------------
	// The global catalog resolves the logical names used across configuration
	// (task-authz rule principals today) to this deployment's token roles and
	// client ids. Validated with the config and injected from here.
	globalCatalog := &cfg.Catalog

	paymentRepo := payment.NewRepository(db)
	paymentRegistry, err := payment.NewRegistry(cfg.Payment, map[string]payment.Factory{
		govpay.MethodID: govpay.NewGovPayGateway,
	})
	if err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to load payment registry: %w", err)
	}
	// Built before the payment service, which takes its references from it, and
	// passed to the task stack for REFID_GENERATOR.
	refIDs, err := initRefIDs(cfg.RefID, db)
	if err != nil {
		_ = database.Close(db)
		return nil, err
	}
	paymentService := payment.NewService(paymentRepo, paymentRegistry, nswpayment.NewRefGenerator(refIDs))
	// Built before payment and storage are handed to the task stack. Workers
	// can emit events as soon as those services exist, so the auditor has to
	// be attached here rather than after the task manager starts. Telemetry
	// above already supplied the OTLP (or nil) logger.
	auditor := nswaudit.New()
	auditor.RegisterSink(nswaudit.NewLogSink(auditLogger))
	coreAuditor := nswaudit.NewCoreAdapter(auditor)
	var unwiredAudit []string
	if !attachAuditor(paymentService, coreAuditor) {
		unwiredAudit = append(unwiredAudit, "payment")
	}

	artifacts, err := initArtifacts(ctx, cfg)
	if err != nil {
		_ = database.Close(db)
		return nil, err
	}

	chaService := cha.NewService(db)
	companyService := company.NewService(db)
	userProfileService := user.NewService(db)

	// Outbound HTTP caller configurations. Set before storage, which in
	// proxy mode reaches the service that owns the files through it.
	remoteManager := remote.NewManager()
	if err := remoteManager.SetServices(cfg.Services...); err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to configure remote services: %w", err)
	}

	// Storage is built here rather than alongside its HTTP handler further
	// down: task plugins that attach uploaded files to an outbound call read
	// through this service, so it has to exist before the task stack (Stage 4).
	// storage.type picks a backend of this deployment's own or a proxy onto
	// the service that owns the files.
	internalUploadTypes, err := internalUploadTypesByClient(cfg.Storage, globalCatalog)
	if err != nil {
		_ = database.Close(db)
		return nil, err
	}
	storageStack, err := nswstorage.New(ctx, cfg.Storage, remoteManager, nswstorage.WithInternalUploadTypes(internalUploadTypes))
	if err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}
	if !attachAuditor(storageStack.Service, coreAuditor) {
		unwiredAudit = append(unwiredAudit, "storage")
	}
	// The file tokens callers hold in place of stored file values.
	fileAccess, err := newFileAccess(cfg.Storage)
	if err != nil {
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to initialize file tokens: %w", err)
	}

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
	// The run ID is "" (Temporal: the workflow's current run) because a parent
	// workflow only ever has one run; the step ID alone names the Activity in it.
	// CompleteActivation's error is returned as it is: the orchestrator treats
	// workflow.ErrActivationNotPending as "an earlier attempt already woke the parent".
	onTaskCompleted := func(parentWorkflowID, parentStepID string, finalVariables map[string]any) error {
		return parentRunner.CompleteActivation(context.Background(), parentWorkflowID, "", parentStepID, finalVariables)
	}

	// OGAs are sent file tokens issued to the machine client each calls back
	// as. An agency doesn't own its files: what it stores are TNSW's tokens,
	// which it sends back to TNSW as they are.
	var ogaFiles taskplugins.OGAFiles
	if !cfg.Storage.IsProxy() {
		ogaFiles = taskplugins.OGAFiles{
			ClientFor: func(serviceID string) (string, bool) {
				clientID, ok := globalCatalog.Clients[serviceID]
				return clientID, ok
			},
			// An OGA opens a file it is sent; it never submits one back.
			IssueForClient: func(clientID, value string) (string, error) {
				return fileAccess.IssueForClient(clientID, value, fileaccess.ScopeRead)
			},
			RenderConfigOf: renderConfigOf(artifacts),
		}
	}
	task, stopTask, err := initTask(db, temporalClient, remoteManager, paymentService, refIDs, companyService, storageStack.Service, ogaFiles, artifacts, globalCatalog, cfg, onTaskCompleted)
	if err != nil {
		temporalClient.Close()
		_ = database.Close(db)
		return nil, err
	}
	tm := task.Manager
	// The payment service and the webhook handlers below complete steps on their
	// own authority, so they reach the task manager as the system principal. The
	// payment service also names no command, so its completer names the step's.
	systemTasks := systemcaller.New(tm)
	paymentService.SetTaskCompleter(nswpayment.NewTaskCompleter(systemTasks))

	// -------------------------------------------------------------------
	// Stage 5: Consignment Service & Workflow Parent Runner
	// -------------------------------------------------------------------
	// The mode decides what starts parent workflows: consignments in TNSW, injects in
	// an agency (see agency.go). An agency builds none of the consignment stack, whose
	// constructors require the trader and CHA roles an agency catalog does not map.
	agencyMode := cfg.Mode == config.ModeAgency
	var (
		consignmentService *consignment.Service
		consignmentRouter  *consignment.Router
		flowService        *caseflow.Service
		upstream           parentUpstreamService
	)
	if agencyMode {
		upstream = agencyCompletion(db)
	} else {
		consignmentService, err = consignment.NewService(db, artifacts, chaService, companyService, userProfileService, task.Store, globalCatalog.Roles, cfg.Consignment.Flows)
		if err != nil {
			_ = stopTask()
			temporalClient.Close()
			_ = database.Close(db)
			return nil, fmt.Errorf("failed to build consignment service: %w", err)
		}
		consignmentRouter, err = consignment.NewRouter(consignmentService, chaService, companyService, auditor, globalCatalog.Roles)
		if err != nil {
			_ = stopTask()
			temporalClient.Close()
			_ = database.Close(db)
			return nil, fmt.Errorf("failed to build consignment router: %w", err)
		}
		// A consignment is a case: caseflow starts its flows and, as the completion
		// callback, keeps its state in step with them.
		sqlDB, err := db.DB()
		if err != nil {
			_ = stopTask()
			temporalClient.Close()
			_ = database.Close(db)
			return nil, fmt.Errorf("failed to get sql.DB for caseflow: %w", err)
		}
		flowService = caseflow.NewService(caseflow.NewRepository(sqlDB), artifacts)
		if err := consignmentService.RegisterCaseFlows(flowService); err != nil {
			_ = stopTask()
			temporalClient.Close()
			_ = database.Close(db)
			return nil, fmt.Errorf("failed to register case flows with consignment service: %w", err)
		}
		upstream = flowService
	}

	pr, stopParentRunner, err := wireParentRunner(temporalClient, cfg.Temporal.Namespace, tm, upstream)
	if err != nil {
		_ = stopTask()
		temporalClient.Close()
		_ = database.Close(db)
		return nil, fmt.Errorf("failed to wire parent runner: %w", err)
	}
	pr.RegisterDefinitionHandler(func(manifestVersion int, templateID string) (workflow.WorkflowDefinition, error) {
		ctx := context.Background()
		reg, err := artifacts.Registry(ctx, manifestVersion)
		if err != nil {
			return workflow.WorkflowDefinition{}, fmt.Errorf("workflow definition %q: %w", templateID, err)
		}
		def, err := workflowdef.Load(ctx, reg, templateID)
		if err != nil {
			return workflow.WorkflowDefinition{}, fmt.Errorf("workflow definition %q not found in manifest version %d: %w", templateID, manifestVersion, err)
		}
		return def, nil
	})
	parentRunner = pr

	if !agencyMode {
		if err := flowService.RegisterWorkflowManager(parentRunner); err != nil {
			_ = stopParentRunner()
			_ = stopTask()
			temporalClient.Close()
			_ = database.Close(db)
			return nil, fmt.Errorf("failed to register workflow manager with flow service: %w", err)
		}
		if err := consignmentService.RegisterWorkflowManager(parentRunner); err != nil {
			_ = stopParentRunner()
			_ = stopTask()
			temporalClient.Close()
			_ = database.Close(db)
			return nil, fmt.Errorf("failed to register workflow manager with consignment service: %w", err)
		}
		if err := consignmentService.RegisterTaskWorkflowManager(task.Runner); err != nil {
			_ = stopParentRunner()
			_ = stopTask()
			temporalClient.Close()
			_ = database.Close(db)
			return nil, fmt.Errorf("failed to register task workflow manager with consignment service: %w", err)
		}
		if err := consignmentService.RegisterTaskCanceller(tm); err != nil {
			_ = stopParentRunner()
			_ = stopTask()
			temporalClient.Close()
			_ = database.Close(db)
			return nil, fmt.Errorf("failed to register task canceller with consignment service: %w", err)
		}
	}

	// -------------------------------------------------------------------
	// Stage 6: Identity Provider (IDP) Authentication Manager
	// -------------------------------------------------------------------
	authnManager, err := nswauthn.NewManager(&authUserProfileAdapter{svc: userProfileService}, cfg.Authn)
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

	// TNSW's trade integrations: customs, port and payment callbacks, and the
	// trader portal's CHA/company lookups. An agency serves none of them, so it builds
	// none of them either — the SLPA handler, for one, refuses to start without its
	// secret.
	var (
		slceHandler    *asycuda.Handler
		slpaHandler    *slpawebhook.Handler
		paymentHandler *payment.HTTPHandler
		chaHandler     *cha.Handler
		companyHandler *company.Handler
	)
	if !agencyMode {
		// ASYCUDA webhook stack.
		cdnRepo := cdn.NewDispatchNoteRepository(db)
		cdnWebhookService := cdn.NewCDNWebhookService(cdnRepo, parkedtask.NewStore(db), systemTasks, caseFlowStarter{flowService})

		cusdecRepo := cusdec.NewDeclarationRepository(db)
		cusdecWebhookService := cusdec.NewWebhookService(cusdecRepo, parkedtask.NewStore(db), systemTasks, caseFlowStarter{flowService})

		slceHandler = asycuda.NewHandler(cusdecWebhookService, cdnWebhookService)

		// TEMPORARY debugging code (see asycuda/debug_forward.go): copy every SLC
		// Edge callback to the slce-debug-forward service when config.yaml has one.
		for _, svc := range cfg.Services {
			if svc.ID == asycuda.DebugForwardServiceID {
				slceHandler.DebugForward = remoteManager
				slog.Warn("slce debug forward is ON: every SLC Edge callback is copied to another deployment", "url", svc.URL)
			}
		}

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
		slpaHandler, err = slpawebhook.NewHandler(
			slpawebhook.NewOrderEvents(db, systemTasks),
			slpawebhook.NewInvoiceEvents(db, systemTasks),
			cfg.Integrations.SLPAWebhook(),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to build the SLPA webhook handler: %w", err)
		}

		chaHandler = cha.NewHandler(chaService)
		companyHandler = company.NewHandler(companyService)
		paymentHandler = payment.NewHTTPHandler(paymentService)
	}

	staticDataHandler := staticdata.NewHandler(artifacts)
	profileHandler := profile.NewHandler(userProfileService, companyService)
	// The storage service behind this handler is built in Stage 2 — task
	// plugins that attach uploaded files to an outbound call read through the
	// service, so it has to exist before the task stack (Stage 4).
	fileHandler := nswstorage.NewFileHandler(storageStack.Service, fileAccess,
		nswstorage.WithUploads(storageStack.Uploads), nswstorage.WithMachineUploads(storageStack.MachineUploads))
	// The catalog is Layer 2 of task authorization on the read path: HandleGetTask
	// decides access from the role-tied ownership of the task's consignment.
	taskFiles := tasks.NewFileBinding(fileAccess, stepNamespace(artifacts), renderConfigOf(artifacts))
	canAct := stepCanAct(artifacts, taskauthzext.NewExtension(taskCatalog(globalCatalog)))
	taskHandler := tasks.NewHTTPHandler(tm, task.Store, task.Assembler, taskCatalog(globalCatalog), taskFiles, canAct, userDisplayNames{svc: userProfileService}, auditor, cfg.Server.MaxRequestBytes)
	// An agency uses task claims: every officer owns every injected workflow, so a
	// claim shows the others who is working a task. TNSW does not: a step is already
	// limited to the trader or CHA company that owns the consignment.
	taskHandler.ClaimsEnabled = agencyMode
	taskHandler.TaskTemplateOf = tasks.TaskTemplates(artifacts)
	// Layer 1 of task authorization, shared by the read and write routes: attach
	// the caller's identity and a lazy ownership resolver for the PRE_RESUME authz
	// extension and the read evaluator to consume. In TNSW a company owns a task in
	// every role it holds on the task's case (case_parties: the consignment's trader
	// and CHA); in an agency an officer owns every injected workflow's tasks (see
	// agency.go).
	var taskAuthzGate taskGate
	if agencyMode {
		taskAuthzGate, err = newAgencyTaskGate(db, globalCatalog.Roles)
	} else {
		taskAuthzGate, err = authzgate.NewMiddleware(caseparty.NewRepository(db), companyIDResolver{svc: companyService})
	}
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
	// The extractor bridges the authn layer (internal/authn) into the generic
	// authz.Principal interface, which nswauthn.ScopePrincipal satisfies
	// structurally — so authz imports nothing from the authn layer, and
	// core/authn stays confined to internal/authn.
	authzr, err := authz.New(func(ctx context.Context) (authz.Principal, bool) {
		p, ok := nswauthn.ScopePrincipalFromContext(ctx)
		if !ok {
			return nil, false
		}
		return p, true
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

		if err := database.HealthCheck(r.Context(), db); err != nil {
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
	mux.Handle("POST /api/v1/tasks/{id}/steps/{stepId}", withAuth(withScope(scopes.TaskWrite)(taskAuthzGate.Handler(http.HandlerFunc(taskHandler.HandleCompleteTaskStep)))))
	mux.Handle("PUT /api/v1/tasks/{id}/steps/{stepId}/draft", withAuth(withScope(scopes.TaskWrite)(taskAuthzGate.Handler(http.HandlerFunc(taskHandler.HandleSaveDraft)))))
	mux.Handle("POST /api/v1/callbacks/{token}", withAuth(withScope(scopes.TaskWrite)(taskAuthzGate.Handler(http.HandlerFunc(taskHandler.HandleCompleteTaskStepByToken)))))
	mux.Handle("POST /api/v1/tasks/{id}/claim", withAuth(withScope(scopes.TaskWrite)(taskAuthzGate.Handler(http.HandlerFunc(taskHandler.HandleClaimTask)))))
	mux.Handle("POST /api/v1/tasks/{id}/release", withAuth(withScope(scopes.TaskWrite)(taskAuthzGate.Handler(http.HandlerFunc(taskHandler.HandleReleaseTask)))))

	mux.Handle("GET /api/v1/static-data/{id}", withAuth(withScope(scopes.TaskRead)(http.HandlerFunc(staticDataHandler.HandleGet))))

	mux.Handle("GET /api/v1/users/me", withAuth(withScope(scopes.ProfileRead)(http.HandlerFunc(profileHandler.HandleGetProfile))))

	// Storage
	mux.Handle(nswstorage.UploadRoute, withAuth(withScope(scopes.StorageWrite)(http.HandlerFunc(fileHandler.Upload))))
	mux.Handle(nswstorage.DownloadRoute, withAuth(withScope(scopes.StorageRead)(http.HandlerFunc(fileHandler.Download))))

	// Mode-specific routes: TNSW's consignment, CHA/company, payment and webhook
	// routes, or the agency's inject and case routes. The shared routes above serve both.
	if agencyMode {
		mountAgency(mux, db, artifacts, parentRunner, task.Store, globalCatalog.Roles, withAuth, withScope)
	} else {
		mountTNSW(mux, tnswHandlers{
			consignment: consignmentRouter,
			cha:         chaHandler,
			company:     companyHandler,
			payment:     paymentHandler,
			slce:        slceHandler,
			slpa:        slpaHandler,
			files:       fileHandler,
		}, withAuth, withScope)
	}

	// When using local storage, these endpoints serve as mocks for S3, at
	// nswstorage.RoutePrefix/{key}/content.
	if localContent := storageStack.LocalContent; localContent != nil {
		localContent.RegisterRoutes(mux)
	}

	// -------------------------------------------------------------------
	// Stage 8: Server Instantiation & Close Hook
	// -------------------------------------------------------------------
	// Security log reads slog.Default (process stream, and OTLP when export
	// is on from the early configureLogging). It sits inside trace so the
	// line carries the trace id, and outside CORS so a preflight is logged
	// too. CaptureRoute next to the mux records Pattern for Middleware:
	// otelhttp copies the request before ServeMux sets it. Authentication
	// attaches the principal on a child request; the log reads it back from
	// authn.RequestCaller.
	for _, name := range unwiredAudit {
		slog.Warn(name + " audit is not wired: service has no WithAuditor")
	}
	handler := trace.TraceMiddleware(securitylog.Middleware(slog.Default())(cors.CORS(&cfg.CORS)(tel.Handler("tnsw-api", securitylog.CaptureRoute(mux)))))
	server := newHTTPServer(cfg.Server, handler)

	closeFn := func() error {
		var closeErrs []error

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

		// Last, so logs emitted while the rest of the process shuts down are
		// still exported.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := tel.Shutdown(shutdownCtx); err != nil {
			closeErrs = append(closeErrs, fmt.Errorf("failed to shut down telemetry: %w", err))
		}
		cancel()

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
// a downstream domain when a parent workflow completes: *caseflow.Service in TNSW,
// agency.CompletionHandler in an agency.
// taskGate is Layer 1 of task authorization on the task routes: authzgate's
// trader/CHA middleware in TNSW, agency.RoleGate in an agency.
type taskGate interface {
	Handler(next http.Handler) http.Handler
}

type parentUpstreamService interface {
	CompletionHandler(workflowID string, finalContext map[string]any) error
}

// newAdminParkLogger builds the AdminParkHandler registered on a Temporal worker. It is the
// only place that currently reacts to a node parking for admin intervention — core's engine
// itself only records the event on the workflow's own in-memory AuditTrail, which nothing
// surfaces proactively. slog.Error here is deliberate: this should be loud and land wherever
// this service's error-level logs are already aggregated/alerted on, since today there is no
// other way to learn a node parked short of already suspecting a specific workflow ID and
// querying its engine status.
//
// scope identifies which runner registered this handler ("top-level workflow" or "task
// workflow"), since that can't be inferred from the payload itself.
func newAdminParkLogger(scope string) func(workflow.AdminParkPayload) error {
	return func(payload workflow.AdminParkPayload) error {
		// TODO: also persist this park event to a DB (keyed on workflow_id/node_id, cleared on
		// resolution) so currently-parked nodes can be queried on demand instead of only being
		// discoverable via these logs.
		slog.Error(scope+" parked for admin intervention",
			"workflow_id", payload.WorkflowID,
			"run_id", payload.RunID,
			"case_id", payload.CaseID,
			"node_id", payload.NodeID,
			"node_type", payload.NodeType,
			"task_template_id", payload.TaskTemplateID,
			"manifest_version", payload.ManifestVersion,
			"cause", payload.Cause,
		)
		return nil
	}
}

// wireParentRunner wires the core/workflow port of workflow.WireParentRunner.
// core ships no wrapper for this, so the wiring is inlined here, the only
// place that needs it.
//
// It starts the Temporal worker that runs macro/parent workflows on
// parentWorkflowQueue. When a parent workflow reaches a Task node, the
// activator's StartTask is invoked to spawn the corresponding task workflow.
// On parent-workflow completion, upstream.CompletionHandler is invoked so
// the workflow's case can advance its state.
func wireParentRunner(c client.Client, namespace string, activator parentTaskActivator, upstream parentUpstreamService) (workflow.TemporalManager, func() error, error) {
	if activator == nil {
		return nil, nil, fmt.Errorf("parent task activator cannot be nil")
	}

	onActivation := func(ctx context.Context, payload workflow.TaskPayload) (map[string]any, error) {
		return activator.StartTask(ctx, payload)
	}

	onCompletion := func(_ context.Context, completion workflow.WorkflowCompletion) error {
		log.Printf("\n[Parent Workflow] Completed. Final state: %v\n", completion.FinalVariables)
		if upstream != nil {
			if err := upstream.CompletionHandler(completion.WorkflowID, completion.FinalVariables); err != nil {
				return fmt.Errorf("upstream completion handler: %w", err)
			}
		}
		return nil
	}

	runner := workflow.NewTemporalManager(c, namespace, parentWorkflowQueue, onActivation, onCompletion)
	runner.RegisterAdminParkHandler(newAdminParkLogger("top-level workflow"))
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

// taskCatalog narrows the global catalog to the slice task authorization needs.
// Both Layer-2 evaluators take the same value, so the mapping lives here rather
// than at each call site.
func taskCatalog(c *catalog.Catalog) taskauthz.Catalog {
	return taskauthz.Catalog{Roles: c.Roles, Clients: c.Clients}
}

// userDisplayNames names a user by the email on their user record, the only name
// the record holds. A user without a record has no name ("").
type userDisplayNames struct{ svc user.Service }

func (n userDisplayNames) GetDisplayName(ctx context.Context, userID string) (string, error) {
	rec, err := n.svc.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, user.ErrUserNotFound) {
			return "", nil
		}
		return "", err
	}
	return rec.Email, nil
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

// caseFlowStarter opens flows on a case for the SLC Edge webhooks, which name
// the flow themselves (a §6.5.4 or §7.6 amendment opens one per amendment).
type caseFlowStarter struct{ flows *caseflow.Service }

func (c caseFlowStarter) StartFlow(ctx context.Context, caseID, templateID, workflowID string, vars map[string]any) error {
	_, err := c.flows.StartFlow(ctx, caseflow.StartRequest{CaseID: caseID, TemplateID: templateID, WorkflowID: workflowID, Vars: vars})
	return err
}

// newFileAccess builds the file tokens this deployment issues from its storage
// settings.
func newFileAccess(cfg nswstorage.Config) (*fileaccess.Access, error) {
	keys, err := filetoken.ParseKeyset(cfg.TokenKeyset)
	if err != nil {
		return nil, fmt.Errorf("storage.tokenKeyset: %w", err)
	}
	codec, err := filetoken.NewCodec(keys, nil)
	if err != nil {
		return nil, err
	}
	return fileaccess.New(codec, cfg.TokenTTL())
}

// renderConfigOf resolves a task's render config through its task template, in the
// manifest version the task runs on.
func renderConfigOf(artifacts *artifact.Set) tasks.RenderConfigOf {
	return func(ctx context.Context, record store.TaskRecord) (json.RawMessage, error) {
		return renderconfig.Load(ctx, artifacts, record)
	}
}

// stepNamespace looks up the output namespace of a task's active step template,
// in the manifest version the task runs on.
func stepNamespace(artifacts *artifact.Set) tasks.NamespaceOf {
	return func(ctx context.Context, record store.TaskRecord) (string, error) {
		reg, err := artifacts.Registry(ctx, record.ManifestVersion)
		if err != nil {
			return "", err
		}
		template, err := steptemplate.Load(ctx, reg, record.ActiveTaskTemplateID)
		if err != nil {
			return "", err
		}
		return template.OutputNamespace, nil
	}
}

// contextStepNamespace returns the output namespace of a step template in the manifest
// version the task manager puts on ctx (artifact.WithManifestVersion), for the
// extensions that run while a step resumes.
func contextStepNamespace(artifacts *artifact.Set) payloadschema.NamespaceOf {
	return func(ctx context.Context, stepTemplateID string) (string, error) {
		version, ok := artifact.ManifestVersionFrom(ctx)
		if !ok {
			return "", fmt.Errorf("step template %q: no artifact manifest version on the context", stepTemplateID)
		}
		reg, err := artifacts.Registry(ctx, version)
		if err != nil {
			return "", err
		}
		template, err := steptemplate.Load(ctx, reg, stepTemplateID)
		if err != nil {
			return "", err
		}
		return template.OutputNamespace, nil
	}
}

// taskManagerOptions makes the task manager refuse to complete or draft a step whose
// template declares no authz extension, so a step that forgets its rule is closed to
// everyone rather than open to anyone.
func taskManagerOptions() []orchestrator.Option {
	return []orchestrator.Option{orchestrator.WithRequiredPreResumeExtensions(taskauthzext.ExtAuthz)}
}

// stepCanAct decides whether a caller may act on a task's active step, by the
// authz rules on its step template, in the manifest version the task runs on.
// A task with no active step template has nothing to act on.
func stepCanAct(artifacts *artifact.Set, ext *taskauthzext.Extension) tasks.CanAct {
	return func(ctx context.Context, record store.TaskRecord, in taskauthz.Input) (bool, error) {
		if record.ActiveTaskTemplateID == "" {
			return false, nil
		}
		reg, err := artifacts.Registry(ctx, record.ManifestVersion)
		if err != nil {
			return false, fmt.Errorf("step template %q: %w", record.ActiveTaskTemplateID, err)
		}
		template, err := steptemplate.Load(ctx, reg, record.ActiveTaskTemplateID)
		if err != nil {
			return false, fmt.Errorf("step template %q: %w", record.ActiveTaskTemplateID, err)
		}
		return ext.MayActOnStep(ctx, &record, in, template)
	}
}

// registryTemplateProvider adapts the artifact manifests to uiprojector's
// TemplateProvider contract and the notify extension's loader. Generic templates
// (JSONForms schemas, markdown bodies, etc.) are resolved through generictemplate.Load
// in the manifest version of the task being rendered or completed, which the task
// manager puts on ctx (artifact.WithManifestVersion). A call without one is a wiring
// fault: falling back to the current version would change a form under an open task.
type registryTemplateProvider struct {
	artifacts *artifact.Set
}

func (p registryTemplateProvider) GetTemplate(ctx context.Context, id string) ([]byte, error) {
	version, ok := artifact.ManifestVersionFrom(ctx)
	if !ok {
		return nil, fmt.Errorf("template %q: no artifact manifest version on the context", id)
	}
	reg, err := p.artifacts.Registry(ctx, version)
	if err != nil {
		return nil, fmt.Errorf("template %q: %w", id, err)
	}
	raw, err := generictemplate.Load(ctx, reg, id)
	if err != nil {
		return nil, fmt.Errorf("template %q not found in manifest version %d: %w", id, version, err)
	}
	return []byte(raw), nil
}

// registryHTMLTemplates adapts the artifact manifests to the HTML document
// generator's HTMLTemplateSource. Like registryTemplateProvider, it resolves each
// html_template in the manifest version the task manager puts on ctx.
type registryHTMLTemplates struct {
	artifacts *artifact.Set
}

func (p registryHTMLTemplates) HTMLTemplate(ctx context.Context, id string) ([]byte, error) {
	version, ok := artifact.ManifestVersionFrom(ctx)
	if !ok {
		return nil, fmt.Errorf("html template %q: no artifact manifest version on the context", id)
	}
	reg, err := p.artifacts.Registry(ctx, version)
	if err != nil {
		return nil, fmt.Errorf("html template %q: %w", id, err)
	}
	return htmltemplate.Load(ctx, reg, id)
}

// initTask consolidates the task-orchestration engine registrations, remote
// configs, and wiring against core/taskflow + core/workflow. It builds and
// starts the orchestration stack on MICRO_WORKFLOW_QUEUE; the returned
// TemporalManager runs the per-task (micro) sub-workflows, while the
// parent/macro workflow runner is owned by the workflow package and wired
// separately (see Stage 5 below).
// registerFlowPlugins installs the plugins this deployment adds on top of the
// task-plugin set: the synchronous transforms that shape a fan-out, the CHA
// writer, the reference ID generator, and the HTML document generator.
//
// A table rather than a run of near-identical registrations, as
// taskplugins.Register does for the same reason — seven of them inline was most
// of initTask's branching, and adding an eighth meant editing a function that
// has nothing else to do with plugins.
func registerFlowPlugins(reg *plugins.Registry, db *gorm.DB, companyService company.Service, refIDs refid.Registry, htmlTemplates taskplugins.HTMLTemplateSource, files taskplugins.DocumentSaver) error {
	entries := []struct {
		taskType string
		plugin   plugins.TaskPlugin
	}{
		{"HSCODE_SPLIT_BUILDER", trade.NewGenericExecutorPlugin(trade.HscodeSplitBuilderFunc)},
		{taskplugins.TaskTypeCDNSplitBuilder, trade.NewGenericExecutorPlugin(taskplugins.CDNSplitBuilderFunc)},
		{taskplugins.TaskTypeSLPAContainerSplitBuilder, trade.NewGenericExecutorPlugin(taskplugins.SLPAContainerSplitBuilderFunc)},
		{taskplugins.TaskTypeSLPAConsolidationResolve, trade.NewGenericExecutorPlugin(taskplugins.SLPAConsolidationResolveFunc)},
		{taskplugins.TaskTypeCDNResultsCollector, trade.NewGenericExecutorPlugin(taskplugins.CDNResultsCollectorFunc)},
		{"CHA_PERSIST_WRITER", trade.NewCHAPersistPlugin(db, companyService)},
		{taskplugins.TaskTypeRefIDGenerator, taskplugins.NewRefIDGeneratorPlugin(refIDs)},
		{taskplugins.TaskTypeHTMLDocumentGenerator, taskplugins.NewHTMLDocumentGeneratorPlugin(htmlTemplates, files)},
	}

	for _, e := range entries {
		if err := reg.Register(e.taskType, e.plugin); err != nil {
			return fmt.Errorf("failed to register %s plugin: %w", e.taskType, err)
		}
	}
	return nil
}

// attachAuditor calls WithAuditor when the service has it. Released payment
// and storage do not. The call sits next to construction, before the task
// stack starts workers that would race a later assignment. The caller logs a
// miss after logging is configured, so the warning reaches stdout and OTLP.
func attachAuditor(svc any, auditor sharedaudit.Auditor) bool {
	setter, ok := svc.(interface {
		WithAuditor(a sharedaudit.Auditor)
	})
	if !ok {
		return false
	}
	setter.WithAuditor(auditor)
	return true
}

func initTask(
	db *gorm.DB,
	temporalClient client.Client,
	remoteManager *remote.Manager,
	paymentService payment.Service,
	refIDs refid.Registry,
	companyService company.Service,
	storageService nswstorage.Service,
	ogaFiles taskplugins.OGAFiles,
	artifacts *artifact.Set,
	globalCatalog *catalog.Catalog,
	cfg *config.Config,
	onTaskCompleted orchestrator.TaskCompletedCallback,
) (*taskStack, func() error, error) {
	// Instantiate flow plugins registry
	pluginsRegistry := plugins.NewRegistry()
	if err := taskplugins.Register(pluginsRegistry, remoteManager, paymentService, storageService, ogaFiles, taskplugins.CustomsSubmissions{
		Declarations:  cusdec.NewDeclarationRepository(db),
		DispatchNotes: cdn.NewDispatchNoteRepository(db),
	}, cfg.Server.ServiceURL); err != nil {
		return nil, nil, fmt.Errorf("failed to register task plugins: %w", err)
	}
	if err := registerFlowPlugins(pluginsRegistry, db, companyService, refIDs, registryHTMLTemplates{artifacts: artifacts}, storageService); err != nil {
		return nil, nil, err
	}

	taskStore := gormstore.New(db)

	// Construct UI projectors and the renderer/zoneview assembler
	projectors := append(uiprojector.DefaultProjectors(), taskrenderer.NewPaymentProjector(), taskrenderer.NewSLPAConsolidationProjector(), taskrenderer.NewSLPASundryProjector())
	uiAssembler, err := uiprojector.NewAssembler(registryTemplateProvider{artifacts: artifacts}, projectors)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build ui assembler: %w", err)
	}
	taskRenderer := zoneview.NewTaskRenderer(uiAssembler)
	zoneAssembler := zoneview.NewZoneViewAssembler(taskRenderer, artifacts)

	var tm *orchestrator.TaskManager

	// Handlers for events on the per-task (micro) sub-workflows running on
	// MICRO_WORKFLOW_QUEUE. Nodes inside a task workflow activate subtasks
	// via tm.StartTaskStep, which dispatches to the matching plugin.
	microActivationHandler := func(ctx context.Context, payload workflow.TaskPayload) (map[string]any, error) {
		log.Printf("\n[Micro Workflow] Step activated: node=%s template=%s\n", payload.NodeID, payload.TaskTemplateID)
		if tm == nil {
			return nil, fmt.Errorf("task manager is not initialized (misconfiguration)")
		}
		res, err := tm.StartTaskStep(ctx, payload)
		return res, keepNonRetryable(err)
	}

	microCompletionHandler := func(ctx context.Context, completion workflow.WorkflowCompletion) error {
		log.Printf("\n[Micro Workflow] Completed. Final state: %v\n", completion.FinalVariables)
		if tm == nil {
			return fmt.Errorf("task manager is not initialized (misconfiguration)")
		}
		return tm.HandleTaskCompletion(ctx, completion)
	}

	workflowRunner := workflow.NewTemporalManager(temporalClient, cfg.Temporal.Namespace, "MICRO_WORKFLOW_QUEUE", microActivationHandler, microCompletionHandler)
	logTaskPark := newAdminParkLogger("task workflow")
	workflowRunner.RegisterAdminParkHandler(func(payload workflow.AdminParkPayload) error {
		_ = logTaskPark(payload)
		if tm == nil {
			return fmt.Errorf("task manager is not initialized (misconfiguration)")
		}
		return tm.HoldTask(context.Background(), payload)
	})

	emailProvider, err := providers.NewEmailProvider(cfg.Notification.Providers.Email)
	if err != nil {
		return nil, nil, fmt.Errorf("email provider: %w", err)
	}
	smsProvider, err := providers.NewSMSProvider(cfg.Notification.Providers.SMS)
	if err != nil {
		return nil, nil, fmt.Errorf("sms provider: %w", err)
	}
	notifManager, err := notification.NewManager(emailProvider, smsProvider)
	if err != nil {
		return nil, nil, fmt.Errorf("notification manager: %w", err)
	}

	extensionsRegistry := extensions.NewRegistry()
	if err := notify.Register(extensionsRegistry, notifManager, registryTemplateProvider{artifacts: artifacts}); err != nil {
		return nil, nil, fmt.Errorf("register notification extension: %w", err)
	}
	if err := taskauthzext.Register(extensionsRegistry, taskCatalog(globalCatalog)); err != nil {
		return nil, nil, fmt.Errorf("register authz extension: %w", err)
	}
	if err := payloadschema.Register(extensionsRegistry, registryTemplateProvider{artifacts: artifacts}.GetTemplate, contextStepNamespace(artifacts)); err != nil {
		return nil, nil, fmt.Errorf("register payload schema extension: %w", err)
	}
	tm = orchestrator.NewTaskManager(taskStore, artifacts, pluginsRegistry, extensionsRegistry, workflowRunner, onTaskCompleted, taskRenderer, taskManagerOptions()...)

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

// initArtifacts creates the artifact loader and loads every published manifest
// (manifest_v1.json, manifest_v2.json, …, or manifest.json as version 1 where there are
// none). New flows start on the highest version, or on artifacts.manifestVersion when it
// pins one.
func initArtifacts(ctx context.Context, cfg *config.Config) (*artifact.Set, error) {
	artifactLoader, err := loaders.New(ctx, cfg.ArtifactLoader)
	if err != nil {
		return nil, fmt.Errorf("failed to create artifact loader: %w", err)
	}

	artifacts, err := artifact.LoadSet(ctx, artifactLoader, artifact.WithPinnedVersion(cfg.Artifacts.ManifestVersion))
	if err != nil {
		return nil, fmt.Errorf("failed to load artifact manifests: %w", err)
	}

	return artifacts, nil
}
