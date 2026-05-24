package activities

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

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
	birthday   string
	sessionID  string
	stage      string
	message    string
	startedAt  int64
	updatedAt  int64
	otpAction  int64
	otpWait    int64
	taskSeq    int64
	otpNeed    bool
	done       bool
	success    bool
	errMessage string
	result     *pb.RegisterResponse

	ctx      context.Context
	cancel   context.CancelFunc
	otpCh    chan string
	doneCh   chan struct{}
	doneOnce sync.Once
}

type browserAuthSession struct {
	sessionToken string
	accessToken  string
}

func newBrowserAuthFlow(mode, jobID string, account *pb.Account) *browserAuthFlow {
	now := time.Now().Unix()
	ctx, cancel := context.WithCancel(context.Background())
	fullName := strings.TrimSpace(strings.Join([]string{account.GetFirstName(), account.GetLastName()}, " "))
	if fullName == "" {
		fullName = browserAuthDefaultRegistrationName
	}
	birthday := strings.TrimSpace(account.GetDob())
	if birthday == "" {
		birthday = browserAuthDefaultBirthday
	}
	return &browserAuthFlow{
		flowID:    uuid.NewString(),
		mode:      strings.TrimSpace(mode),
		jobID:     strings.TrimSpace(jobID),
		email:     strings.TrimSpace(account.GetEmail()),
		password:  account.GetPassword(),
		fullName:  fullName,
		birthday:  birthday,
		stage:     browserAuthStageQueued,
		message:   "browser auth queued",
		startedAt: now,
		updatedAt: now,
		ctx:       ctx,
		cancel:    cancel,
		otpCh:     make(chan string, 1),
		doneCh:    make(chan struct{}),
	}
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

func (s *Server) browserAuthStart(ctx context.Context, mode, jobID string, account *pb.Account) (*pb.StartRegisterResponse, error) {
	if s.browserAutomationClient == nil {
		return nil, fmt.Errorf("browser automation client is not configured")
	}
	if account == nil {
		return nil, fmt.Errorf("account is required")
	}
	flow := newBrowserAuthFlow(mode, jobID, account)
	s.browserAuthFlows.add(flow)
	go flow.run(s.browserAutomationClient, s.browserAuthConfig)
	return flow.startResponse(), nil
}

func (s *Server) browserAuthComplete(ctx context.Context, mode, flowID, otp string) (*pb.RegisterResponse, error) {
	flow := s.browserAuthFlows.get(flowID)
	if flow == nil {
		return &pb.RegisterResponse{Success: false, ErrorMessage: fmt.Sprintf("browser %s flow not found", mode)}, nil
	}
	resp, err := flow.complete(ctx, otp)
	if err == nil {
		s.browserAuthFlows.remove(flowID)
	}
	return resp, err
}

func (s *Server) browserAuthResendOTP(ctx context.Context, mode, flowID string) (*pb.BrowserAuthResendOTPOutput, error) {
	if s.browserAutomationClient == nil {
		return nil, fmt.Errorf("browser automation client is not configured")
	}
	flow := s.browserAuthFlows.get(flowID)
	if flow == nil {
		return &pb.BrowserAuthResendOTPOutput{
			FlowId:       flowID,
			Success:      false,
			ErrorMessage: fmt.Sprintf("browser %s flow not found", mode),
		}, nil
	}
	return flow.resendEmailOTP(s.browserAutomationClient, s.browserAuthConfig)
}

func (s *Server) browserAuthStatus(ctx context.Context, flowID string) (*pb.BrowserFlowStatusResponse, error) {
	flow := s.browserAuthFlows.get(flowID)
	if flow == nil {
		return &pb.BrowserFlowStatusResponse{Found: false, FlowId: flowID, ErrorMessage: "browser flow not found"}, nil
	}
	return flow.statusResponse(), nil
}

func (s *Server) browserAuthCancel(ctx context.Context, mode, flowID string) (*pb.CancelRegisterResponse, error) {
	flow := s.browserAuthFlows.get(flowID)
	if flow == nil {
		return &pb.CancelRegisterResponse{Success: true}, nil
	}
	flow.cancelFlow(fmt.Sprintf("browser %s cancelled", mode))
	s.browserAuthFlows.remove(flowID)
	return &pb.CancelRegisterResponse{Success: true}, nil
}
