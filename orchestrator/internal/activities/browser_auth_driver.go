package activities

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	browserautomationv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/browserautomation/v1"
	"github.com/google/uuid"
	"orchestrator/pb"
)

type browserAuthFlow struct {
	mu         sync.Mutex
	flowID     string
	taskScope  string
	mode       string
	jobID      string
	email      string
	password   string
	fullName   string
	age        string
	sessionID  string
	stage      string
	message    string
	startedAt  int64
	updatedAt  int64
	otpAction  int64
	otpWait    int64
	otpKind    string
	taskSeq    int64
	otpNeed    bool
	done       bool
	success    bool
	errMessage string
	result     *pb.RegisterResponse

	ctx    context.Context
	cancel context.CancelFunc
}

type browserAuthSession struct {
	sessionToken string
	accessToken  string
}

func newBrowserAuthFlow(mode, jobID string, account *pb.Account) *browserAuthFlow {
	return newBrowserAuthFlowWithContext(context.Background(), mode, jobID, account)
}

func newBrowserAuthFlowWithContext(ctx context.Context, mode, jobID string, account *pb.Account) *browserAuthFlow {
	now := time.Now().Unix()
	if ctx == nil {
		ctx = context.Background()
	}
	flowCtx, cancel := context.WithCancel(ctx)
	return &browserAuthFlow{
		flowID:    uuid.NewString(),
		mode:      strings.TrimSpace(mode),
		jobID:     strings.TrimSpace(jobID),
		email:     strings.TrimSpace(account.GetEmail()),
		password:  account.GetPassword(),
		fullName:  browserAuthRandomDisplayName(),
		age:       browserAuthRandomAge(),
		stage:     browserAuthStageQueued,
		message:   "browser auth queued",
		startedAt: now,
		updatedAt: now,
		ctx:       flowCtx,
		cancel:    cancel,
	}
}

func newBrowserAuthSessionFlow(ctx context.Context, mode, jobID string, account *pb.Account, sessionID string) *browserAuthFlow {
	flow := newBrowserAuthFlowWithContext(ctx, mode, jobID, account)
	flow.flowID = strings.TrimSpace(sessionID)
	flow.sessionID = strings.TrimSpace(sessionID)
	return flow
}

func (f *browserAuthFlow) setTaskScope(scope string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.taskScope = strings.TrimSpace(scope)
}

func (f *browserAuthFlow) getTaskScope() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.taskScope
}

func (s *Server) browserAuthStart(ctx context.Context, mode, jobID string, account *pb.Account) (*pb.StartRegisterResponse, string, error) {
	if s.browserAutomationClient == nil {
		return nil, "", fmt.Errorf("browser automation client is not configured")
	}
	if account == nil {
		return nil, "", fmt.Errorf("account is required")
	}
	flow := newBrowserAuthFlowWithContext(ctx, mode, jobID, account)
	cfg, err := s.browserAuthConfigForAccount(ctx, account)
	if err != nil {
		return nil, "", err
	}
	if err := flow.runUntilCheckpoint(s.browserAutomationClient, cfg); err != nil {
		flow.stopSession(s.browserAutomationClient)
		return flow.startResponse(), flow.getOTPKind(), err
	}
	if flow.isDone() {
		flow.stopSession(s.browserAutomationClient)
	}
	return flow.startResponse(), flow.getOTPKind(), nil
}

func (s *Server) browserAuthComplete(ctx context.Context, mode, jobID string, account *pb.Account, flowID, otp, otpKind string) (*pb.RegisterResponse, error) {
	if s.browserAutomationClient == nil {
		return nil, fmt.Errorf("browser automation client is not configured")
	}
	if account == nil {
		return nil, fmt.Errorf("account is required")
	}
	flow := newBrowserAuthSessionFlow(ctx, mode, jobID, account, flowID)
	flow.setTaskScope("complete")
	defer flow.stopSession(s.browserAutomationClient)
	if err := flow.completeFromCheckpoint(s.browserAutomationClient, s.browserAuthConfig, otp, otpKind); err != nil {
		flow.fail(err)
		return flow.registerResponse(), err
	}
	return flow.registerResponse(), nil
}

func (s *Server) browserAuthResendOTP(ctx context.Context, mode, jobID string, account *pb.Account, flowID, otpKind string) (*pb.BrowserAuthResendOTPOutput, error) {
	if s.browserAutomationClient == nil {
		return nil, fmt.Errorf("browser automation client is not configured")
	}
	if account == nil {
		return nil, fmt.Errorf("account is required")
	}
	if otpKind == browserAuthOTPKindLoginEmail {
		return &pb.BrowserAuthResendOTPOutput{
			BrowserSessionId: flowID,
			Email:            account.GetEmail(),
			Success:          false,
			ErrorMessage:     fmt.Sprintf("browser %s login email OTP resend is not supported", mode),
		}, nil
	}
	flow := newBrowserAuthSessionFlow(ctx, mode, jobID, account, flowID)
	flow.setTaskScope(fmt.Sprintf("resend-%d", time.Now().UnixMilli()))
	return flow.resendEmailOTP(s.browserAutomationClient, s.browserAuthConfig)
}

func (s *Server) browserAuthCancel(ctx context.Context, mode, flowID string) (*pb.CancelRegisterResponse, error) {
	if s.browserAutomationClient == nil || strings.TrimSpace(flowID) == "" {
		return &pb.CancelRegisterResponse{Success: true}, nil
	}
	_, err := s.browserAutomationClient.StopBrowserSession(ctx, &browserautomationv1.StopBrowserSessionRequest{
		SessionId: strings.TrimSpace(flowID),
		Reason:    fmt.Sprintf("browser %s cancelled", mode),
	})
	if err != nil {
		return nil, err
	}
	return &pb.CancelRegisterResponse{Success: true}, nil
}
