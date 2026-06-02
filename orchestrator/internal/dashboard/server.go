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

	"orchestrator/internal/accountfingerprint"
	"orchestrator/internal/accountproxyusage"
	"orchestrator/internal/actionregistry"
	"orchestrator/internal/contracts"
	"orchestrator/internal/runtimesecrets"
	"orchestrator/pb"
)

type Config struct {
	ListenAddr         string
	N8NWebhookBaseURL  string
	N8NActions         N8NActions
	AccountClient      pb.GPTAccountServiceClient
	PaymentClient      pb.PaymentServiceClient
	RuntimeSecrets     runtimesecrets.Store
	Fingerprints       *accountfingerprint.Store
	AccountProxyUsages accountProxyUsageStore
	WorkflowAPI        WorkflowAPI
	Settings           SettingsStore
	HotStream          hotstream.Subscriber
	ActionRegistry     *actionregistry.Registry
}

type SettingsStore interface {
	Get(ctx context.Context) (*pb.GPTSettings, error)
	Update(ctx context.Context, settings *pb.GPTSettings) (*pb.GPTSettings, error)
}

type accountProxyUsageStore interface {
	ListByAccount(context.Context, string, int) ([]accountproxyusage.Usage, error)
}

type WorkflowAPI interface {
	CreateGPTAccount(context.Context, *pb.CreateGPTAccountRequest) (*pb.CreateGPTAccountResponse, error)
	FetchAccountMailbox(context.Context, *pb.FetchAccountMailboxRequest) (*pb.FetchAccountMailboxResponse, error)
	SubmitOTP(context.Context, *pb.SubmitOTPRequest) (*pb.SubmitOTPResponse, error)
	ResendOTP(context.Context, *pb.ResendOTPRequest) (*pb.ResendOTPResponse, error)
	CancelJob(context.Context, *pb.CancelJobRequest) (*pb.CancelJobResponse, error)
	ListJobs(context.Context, *pb.ListJobsRequest) (*pb.ListJobsResponse, error)
	GetJob(context.Context, *pb.GetJobRequest) (*pb.GetJobResponse, error)
}

type server struct {
	accountClient      pb.GPTAccountServiceClient
	workflowAPI        WorkflowAPI
	paymentClient      pb.PaymentServiceClient
	runtimeSecrets     runtimesecrets.Store
	fingerprints       *accountfingerprint.Store
	accountProxyUsages accountProxyUsageStore
	settings           SettingsStore
	n8nWorkflows       map[string]*n8nWebhookClient
	n8nWebhookBaseURL  string
	n8nActions         N8NActions
	hotstream          hotstream.Subscriber
	staticDir          string
	actionRegistry     *actionregistry.Registry
}

const gptDashboardStaticDir = "/app/dashboard/gpt"

type Server struct {
	httpServer *http.Server
	listener   net.Listener
}

type N8NActions interface {
	gptplugin.N8NActionInvoker
	gptplugin.N8NWorkflowStarter
	N8NProbeActions
	N8NCodexOAuthActions
	N8NCodexOAuthProtocolActions
	N8NCodexOAuthAddPhoneActions
	N8NCodexOAuthBatchActions
	N8NRegisterActions
	N8NRegisterProtocolActions
	N8NLoginSessionActions
	N8NLoginSessionProtocolActions
}

func Start(ctx context.Context, cfg Config) (*Server, error) {
	if strings.TrimSpace(cfg.ListenAddr) == "" {
		return nil, nil
	}
	if cfg.WorkflowAPI == nil {
		return nil, errors.New("workflow API is required")
	}
	actionRegistry := cfg.ActionRegistry
	if actionRegistry == nil {
		return nil, errors.New("action registry is required")
	}
	s := &server{
		accountClient:      cfg.AccountClient,
		workflowAPI:        cfg.WorkflowAPI,
		paymentClient:      cfg.PaymentClient,
		runtimeSecrets:     cfg.RuntimeSecrets,
		fingerprints:       cfg.Fingerprints,
		accountProxyUsages: cfg.AccountProxyUsages,
		settings:           cfg.Settings,
		n8nWorkflows:       catalogN8NWebhookClients(actionRegistry, cfg.N8NWebhookBaseURL),
		n8nWebhookBaseURL:  cfg.N8NWebhookBaseURL,
		n8nActions:         cfg.N8NActions,
		hotstream:          cfg.HotStream,
		staticDir:          gptDashboardStaticDir,
		actionRegistry:     actionRegistry,
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

func catalogN8NWebhookClients(registry *actionregistry.Registry, baseURL string) map[string]*n8nWebhookClient {
	out := map[string]*n8nWebhookClient{}
	for _, def := range registry.Actions() {
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
	return s.n8nWorkflows[contracts.NormalizeActionID(actionID)]
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
		{"/otp", s.submitChannelOTP},
		{"/streams/state", s.streamState},
		{"/jobs/", s.handleJob},
	}
	routes = append(routes, s.privateRouteBindings()...)
	routes = append(routes, s.n8nActionRouteBindings()...)
	return append(routes, s.workflowRouteBindings()...)
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
