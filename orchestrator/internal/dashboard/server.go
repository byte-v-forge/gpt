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
	"google.golang.org/grpc"
	"gorm.io/gorm"

	"orchestrator/internal/accountfingerprint"
	"orchestrator/internal/runtimesecrets"
	"orchestrator/pb"
)

type Config struct {
	ListenAddr                 string
	N8NWebhookBaseURL          string
	N8NProbeActions            N8NProbeActions
	N8NRegisterProtocolActions N8NRegisterProtocolActions
	AccountClient              pb.GPTAccountServiceClient
	PaymentClient              pb.PaymentServiceClient
	RuntimeSecrets             runtimesecrets.Store
	Fingerprints               *accountfingerprint.Store
	DB                         *gorm.DB
	Settings                   SettingsStore
	WorkflowConn               grpc.ClientConnInterface
	HotStream                  hotstream.Subscriber
}

type SettingsStore interface {
	Get(ctx context.Context) (*pb.GPTSettings, error)
	Update(ctx context.Context, settings *pb.GPTSettings) (*pb.GPTSettings, error)
}

type server struct {
	accountClient              pb.GPTAccountServiceClient
	accountWorkflowClient      pb.AccountWorkflowServiceClient
	paymentWorkflowClient      pb.PaymentWorkflowServiceClient
	gopayAppClient             pb.GoPayAppWorkflowServiceClient
	otpClient                  pb.OTPServiceClient
	jobClient                  pb.JobServiceClient
	paymentClient              pb.PaymentServiceClient
	runtimeSecrets             runtimesecrets.Store
	fingerprints               *accountfingerprint.Store
	db                         *gorm.DB
	settings                   SettingsStore
	n8nProbe                   *n8nWebhookClient
	n8nRegisterProtocol        *n8nWebhookClient
	n8nProbeActions            N8NProbeActions
	n8nRegisterProtocolActions N8NRegisterProtocolActions
	proxyRuntimeProxy          http.Handler
	hotstream                  hotstream.Subscriber
	staticDir                  string
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
	s := &server{
		accountClient:              cfg.AccountClient,
		accountWorkflowClient:      pb.NewAccountWorkflowServiceClient(cfg.WorkflowConn),
		paymentWorkflowClient:      pb.NewPaymentWorkflowServiceClient(cfg.WorkflowConn),
		gopayAppClient:             pb.NewGoPayAppWorkflowServiceClient(cfg.WorkflowConn),
		otpClient:                  pb.NewOTPServiceClient(cfg.WorkflowConn),
		jobClient:                  pb.NewJobServiceClient(cfg.WorkflowConn),
		paymentClient:              cfg.PaymentClient,
		runtimeSecrets:             cfg.RuntimeSecrets,
		fingerprints:               cfg.Fingerprints,
		db:                         cfg.DB,
		settings:                   cfg.Settings,
		n8nProbe:                   newN8NWebhookClient("probe-account", cfg.N8NWebhookBaseURL, "gpt/probe-account"),
		n8nRegisterProtocol:        newN8NWebhookClient("register-protocol", cfg.N8NWebhookBaseURL, "gpt/register-protocol"),
		n8nProbeActions:            cfg.N8NProbeActions,
		n8nRegisterProtocolActions: cfg.N8NRegisterProtocolActions,
		hotstream:                  cfg.HotStream,
		staticDir:                  gptDashboardStaticDir,
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
	return []routeBinding{
		{"/api/health", s.handleHealth},
		{"/settings", s.handleSettings},
		{"/accounts", s.handleAccounts},
		{"/accounts/", s.handleAccount},
		{"/email-allocations", s.handleGPTEmailAllocations},
		{"/fingerprints/preview", s.handleFingerprintPreview},
		{"/jobs", s.handleJobs},
		{"/streams/state", s.streamState},
		{"/jobs/", s.handleJob},
		{"/actions/probe-account/", s.handleProbeAccountAction},
		{"/actions/register-protocol/", s.handleRegisterProtocolAction},
		{"/gopay/state", s.handleGoPayState},
		{"/gopay/profile", s.handleGoPayProfile},
		{"/gopay/user/", s.handleGoPayUserAction},
		{"/workflows/register-protocol", s.handleRegisterProtocol},
		{"/workflows/register", s.handleRegister},
		{"/workflows/activate", s.handleActivate},
		{"/workflows/autopay", s.handleAutopay},
		{"/workflows/login-protocol", s.handleLoginProtocol},
		{"/workflows/login", s.handleLogin},
		{"/workflows/codex-oauth", s.handleCodexOAuth},
		{"/workflows/codex-oauth-protocol", s.handleCodexOAuthProtocol},
		{"/workflows/codex-oauth-add-phone/batch", s.handleCodexOAuthBatchAddPhone},
		{"/workflows/codex-oauth-add-phone", s.handleCodexOAuthAddPhone},
		{"/workflows/probe", s.handleProbeAccount},
		{"/workflows/gopay-app", s.handleGoPayApp},
		{"/workflows/gopay-qris-payment-activate", s.handleGoPayQRISPaymentActivate},
		{"/workflows/gopay-wa-payment", s.handleGoPayWAPayment},
		{"/workflows/gopay-payment/rebind", s.handleGoPayPaymentRebind},
		{"/workflows/gopay-payment", s.handleGoPayPayment},
		{"/workflows/register-and-activate", s.handleRegisterAndActivate},
	}
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
