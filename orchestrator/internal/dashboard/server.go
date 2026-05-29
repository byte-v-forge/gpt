package dashboard

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/hotstream"
	"github.com/byte-v-forge/common-lib/httpsse"
	"github.com/byte-v-forge/gpt/pkg/gptplugin"
	"google.golang.org/grpc"
	"gorm.io/gorm"

	"orchestrator/internal/accountfingerprint"
	"orchestrator/internal/actionregistry"
	"orchestrator/internal/contracts"
	"orchestrator/internal/runtimesecrets"
	"orchestrator/pb"
)

type Config struct {
	ListenAddr                     string
	N8NWebhookBaseURL              string
	N8NProbeActions                N8NProbeActions
	N8NCodexOAuthActions           N8NCodexOAuthActions
	N8NCodexOAuthProtocolActions   N8NCodexOAuthProtocolActions
	N8NCodexOAuthAddPhoneActions   N8NCodexOAuthAddPhoneActions
	N8NCodexOAuthBatchActions      N8NCodexOAuthBatchActions
	N8NRegisterActions             N8NRegisterActions
	N8NRegisterProtocolActions     N8NRegisterProtocolActions
	N8NLoginSessionActions         N8NLoginSessionActions
	N8NLoginSessionProtocolActions N8NLoginSessionProtocolActions
	N8NActionInvoker               gptplugin.N8NActionInvoker
	N8NWorkflowStarter             gptplugin.N8NWorkflowStarter
	AccountClient                  pb.GPTAccountServiceClient
	PaymentClient                  pb.PaymentServiceClient
	RuntimeSecrets                 runtimesecrets.Store
	Fingerprints                   *accountfingerprint.Store
	DB                             *gorm.DB
	Settings                       SettingsStore
	WorkflowConn                   grpc.ClientConnInterface
	HotStream                      hotstream.Subscriber
	ActionRegistry                 *actionregistry.Registry
}

type SettingsStore interface {
	Get(ctx context.Context) (*pb.GPTSettings, error)
	Update(ctx context.Context, settings *pb.GPTSettings) (*pb.GPTSettings, error)
}

type server struct {
	accountClient                  pb.GPTAccountServiceClient
	accountWorkflowClient          pb.AccountWorkflowServiceClient
	paymentWorkflowClient          pb.PaymentWorkflowServiceClient
	gopayAppClient                 pb.GoPayAppWorkflowServiceClient
	otpClient                      pb.OTPServiceClient
	jobClient                      pb.JobServiceClient
	paymentClient                  pb.PaymentServiceClient
	runtimeSecrets                 runtimesecrets.Store
	fingerprints                   *accountfingerprint.Store
	db                             *gorm.DB
	settings                       SettingsStore
	n8nWorkflows                   map[string]*n8nWebhookClient
	n8nProbeActions                N8NProbeActions
	n8nCodexOAuthActions           N8NCodexOAuthActions
	n8nCodexOAuthProtocolActions   N8NCodexOAuthProtocolActions
	n8nCodexOAuthAddPhoneActions   N8NCodexOAuthAddPhoneActions
	n8nCodexOAuthBatchActions      N8NCodexOAuthBatchActions
	n8nRegisterActions             N8NRegisterActions
	n8nRegisterProtocolActions     N8NRegisterProtocolActions
	n8nLoginSessionActions         N8NLoginSessionActions
	n8nLoginSessionProtocolActions N8NLoginSessionProtocolActions
	n8nActionInvoker               gptplugin.N8NActionInvoker
	n8nWorkflowStarter             gptplugin.N8NWorkflowStarter
	proxyRuntimeProxy              http.Handler
	hotstream                      hotstream.Subscriber
	staticDir                      string
	actionRegistry                 *actionregistry.Registry
}

const gptDashboardStaticDir = "/app/dashboard/gpt"

type Server struct {
	httpServer *http.Server
	listener   net.Listener
}

func Start(ctx context.Context, cfg Config) (*Server, error) {
	if strings.TrimSpace(cfg.ListenAddr) == "" {
		return nil, nil
	}
	if cfg.WorkflowConn == nil {
		return nil, errors.New("workflow connection is required")
	}
	actionRegistry := actionregistry.RegisterDefault(cfg.ActionRegistry)
	s := &server{
		accountClient:                  cfg.AccountClient,
		accountWorkflowClient:          pb.NewAccountWorkflowServiceClient(cfg.WorkflowConn),
		paymentWorkflowClient:          pb.NewPaymentWorkflowServiceClient(cfg.WorkflowConn),
		gopayAppClient:                 pb.NewGoPayAppWorkflowServiceClient(cfg.WorkflowConn),
		otpClient:                      pb.NewOTPServiceClient(cfg.WorkflowConn),
		jobClient:                      pb.NewJobServiceClient(cfg.WorkflowConn),
		paymentClient:                  cfg.PaymentClient,
		runtimeSecrets:                 cfg.RuntimeSecrets,
		fingerprints:                   cfg.Fingerprints,
		db:                             cfg.DB,
		settings:                       cfg.Settings,
		n8nWorkflows:                   catalogN8NWebhookClients(actionRegistry, cfg.N8NWebhookBaseURL),
		n8nProbeActions:                cfg.N8NProbeActions,
		n8nCodexOAuthActions:           cfg.N8NCodexOAuthActions,
		n8nCodexOAuthProtocolActions:   cfg.N8NCodexOAuthProtocolActions,
		n8nCodexOAuthAddPhoneActions:   cfg.N8NCodexOAuthAddPhoneActions,
		n8nCodexOAuthBatchActions:      cfg.N8NCodexOAuthBatchActions,
		n8nRegisterActions:             cfg.N8NRegisterActions,
		n8nRegisterProtocolActions:     cfg.N8NRegisterProtocolActions,
		n8nLoginSessionActions:         cfg.N8NLoginSessionActions,
		n8nLoginSessionProtocolActions: cfg.N8NLoginSessionProtocolActions,
		n8nActionInvoker:               cfg.N8NActionInvoker,
		n8nWorkflowStarter:             cfg.N8NWorkflowStarter,
		hotstream:                      cfg.HotStream,
		staticDir:                      gptDashboardStaticDir,
		actionRegistry:                 actionRegistry,
	}
	mux := http.NewServeMux()
	mux.Handle("/api/gpt/", http.StripPrefix("/api/gpt", s.routes()))
	mux.Handle("/mf/gpt/", http.StripPrefix("/mf/gpt/", noCacheFileServer(s.staticDir)))
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/api/health", s.handleHealth)

	listener, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: withCORS(mux), ReadHeaderTimeout: 5 * time.Second}
	result := &Server{httpServer: server, listener: listener}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go func() {
		log.Printf("GPT dashboard BFF listening on %s", cfg.ListenAddr)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("GPT dashboard BFF stopped: %v", err)
		}
	}()
	return result, nil
}

func newCatalogN8NWebhookClient(registry *actionregistry.Registry, actionID string, baseURL string) *n8nWebhookClient {
	def, ok := actionregistry.RegisterDefault(registry).Action(actionID)
	if !ok || def.Engine != actionregistry.EngineN8N {
		return nil
	}
	return newN8NWebhookClient(def.Workflow.Key, baseURL, def.Workflow.N8NWebhookPath)
}

func catalogN8NWebhookClients(registry *actionregistry.Registry, baseURL string) map[string]*n8nWebhookClient {
	out := map[string]*n8nWebhookClient{}
	for _, def := range actionregistry.RegisterDefault(registry).Actions() {
		if def.Engine != actionregistry.EngineN8N {
			continue
		}
		client := newN8NWebhookClient(def.Workflow.Key, baseURL, def.Workflow.N8NWebhookPath)
		if client != nil {
			out[def.ActionID] = client
		}
	}
	return out
}

func (s *server) n8nWorkflow(actionID string) *n8nWebhookClient {
	if s == nil {
		return nil
	}
	return s.n8nWorkflows[strings.ToUpper(strings.TrimSpace(actionID))]
}

func (s *Server) Close() error {
	if s == nil || s.httpServer == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	for _, route := range s.routeBindings() {
		mux.HandleFunc(route.path, route.handler)
	}
	return mux
}

func (s *server) streamState(w http.ResponseWriter, r *http.Request) {
	httpsse.ServeHotStream(w, r, s.hotstream, httpsse.FilterFromRequest(r, hotstream.Filter{
		SourceServices: []string{"gpt-orchestrator", "gpt-account"},
	}), httpsse.ServeOptions{})
}

type routeBinding struct {
	path    string
	handler http.HandlerFunc
}

func (s *server) routeBindings() []routeBinding {
	routes := []routeBinding{
		{"/api/health", s.handleHealth},
		{"/settings", s.handleSettings},
		{"/action-catalog", s.handleActionCatalog},
		{"/accounts", s.handleAccounts},
		{"/accounts/", s.handleAccount},
		{"/email-allocations", s.handleGPTEmailAllocations},
		{"/fingerprints/preview", s.handleFingerprintPreview},
		{"/jobs", s.handleJobs},
		{"/streams/state", s.streamState},
		{"/jobs/", s.handleJob},
	}
	if s.goPayActionsEnabled() {
		routes = append(routes,
			routeBinding{"/gopay/state", s.handleGoPayState},
			routeBinding{"/gopay/profile", s.handleGoPayProfile},
			routeBinding{"/gopay/user/", s.handleGoPayUserAction},
		)
	}
	routes = append(routes, s.n8nActionRouteBindings()...)
	return append(routes, s.workflowRouteBindings()...)
}

func (s *server) goPayActionsEnabled() bool {
	if s == nil || s.actionRegistry == nil {
		return false
	}
	return s.actionRegistry.HasCapability(contracts.CapabilityGoPay)
}

func (s *server) n8nActionRouteBindings() []routeBinding {
	handlers := s.n8nActionHandlers()
	out := make([]routeBinding, 0, len(handlers))
	for _, def := range s.actionRegistry.Actions() {
		handler := handlers[def.ActionID]
		path := strings.TrimRight(strings.TrimSpace(def.Workflow.ActionPathPrefix), "/")
		if handler == nil || path == "" {
			continue
		}
		out = append(out, routeBinding{path + "/", handler})
	}
	return out
}

func (s *server) actionSubPath(r *http.Request, actionID string) string {
	def, ok := s.actionRegistry.Action(actionID)
	if !ok {
		return ""
	}
	prefix := strings.TrimRight(strings.TrimSpace(def.Workflow.ActionPathPrefix), "/") + "/"
	return strings.Trim(strings.TrimPrefix(r.URL.Path, prefix), "/")
}

func (s *server) workflowRouteBindings() []routeBinding {
	handlers := s.workflowHandlers()
	out := make([]routeBinding, 0, len(handlers))
	for _, def := range s.actionRegistry.Actions() {
		handler := handlers[def.ActionID]
		if handler == nil || strings.TrimSpace(def.Workflow.StartPath) == "" {
			continue
		}
		out = append(out, routeBinding{def.Workflow.StartPath, handler})
	}
	return out
}

func noCacheFileServer(dir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		path := filepath.Join(dir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			http.ServeFile(w, r, path)
			return
		}
		http.NotFound(w, r)
	})
}
