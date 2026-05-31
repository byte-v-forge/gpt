package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/contracts"
	"orchestrator/pb"
)

type n8nCodexOAuthStartConfig[Req any, Resp any] struct {
	ActionID  string
	Response  n8nStartResponseBuilder[Resp]
	AccountID func(Req) (string, error)
	Params    func(Req) (map[string]string, error)
}

func (s *Server) StartN8NCodexOAuthAccount(ctx context.Context, actionID string, req *pb.CodexOAuthRequest) (*pb.CodexOAuthResponse, string, error) {
	return startN8NCodexOAuthAction(ctx, s, n8nCodexOAuthStartConfig[*pb.CodexOAuthRequest, *pb.CodexOAuthResponse]{
		ActionID:  actionID,
		Response:  n8nCodexOAuthStartResponse,
		AccountID: codexOAuthRequestAccountID[*pb.CodexOAuthRequest],
		Params: func(req *pb.CodexOAuthRequest) (map[string]string, error) {
			return codexOAuthJobParams(req.GetAccountId(), req.GetLabel()), nil
		},
	}, req)
}

func (s *Server) BindN8NCodexOAuthExecution(ctx context.Context, req *pb.N8NCodexOAuthBindRequest) (any, error) {
	return s.bindN8NExecutionAction(ctx, req.GetJobId(), req.GetN8NExecutionId())
}

func (s *Server) CompleteN8NCodexOAuthAction(ctx context.Context, actionID string, req *pb.N8NCodexOAuthCompleteRequest) (any, error) {
	profile, err := n8nCodexOAuthProfileForAction(actionID)
	if err != nil {
		return nil, err
	}
	return s.completeN8NCodexOAuthAction(ctx, profile, req)
}

func (s *Server) FailN8NCodexOAuthAction(ctx context.Context, actionID string, req *pb.N8NCodexOAuthFailRequest) (any, error) {
	profile, err := n8nCodexOAuthProfileForAction(actionID)
	if err != nil {
		return nil, err
	}
	scope := n8nActionScopeFrom(req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId())
	return s.failBoundN8NActionMessage(ctx, profile.Fail, scope.JobID, scope.AccountID, scope.N8NExecutionID, req.GetStep(), req.GetErrorMessage(), n8nCodexOAuthFailureData(scope, profile, req))
}

func (s *Server) CompleteN8NCodexOAuthBatchAction(ctx context.Context, actionID string, req *pb.N8NCodexOAuthBatchCompleteRequest) (any, error) {
	profile, err := n8nCodexOAuthProfileForAction(actionID)
	if err != nil {
		return nil, err
	}
	scope := n8nActionScopeFrom(req.GetJobId(), "", req.GetN8NExecutionId())
	if err := s.bindN8NExecution(ctx, scope.JobID, scope.N8NExecutionID); err != nil {
		return nil, err
	}
	data := &pb.N8NCodexOAuthBatchCompleteData{
		N8NExecutionId: scope.N8NExecutionID,
		TotalCount:     req.GetTotalCount(),
		Mode:           strings.TrimSpace(req.GetMode()),
	}
	if err := s.storeN8NActionSuccessMessage(ctx, scope.JobID, data); err != nil {
		return nil, err
	}
	return n8nActionCompleteOutcomeMessage(scope.JobID, scope.AccountID, scope.N8NExecutionID, profile.Complete.Action, profile.Complete.Started, true, "", data), nil
}

func (s *Server) completeN8NCodexOAuthAction(ctx context.Context, profile n8nCodexOAuthProfile, req *pb.N8NCodexOAuthCompleteRequest) (any, error) {
	scope := n8nActionScopeFrom(req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId())
	if err := s.bindN8NExecution(ctx, scope.JobID, scope.N8NExecutionID); err != nil {
		return nil, err
	}
	data := n8nCodexOAuthCompleteData(scope, profile, req)
	if err := s.storeN8NActionSuccessMessage(ctx, scope.JobID, data); err != nil {
		return nil, err
	}
	cfg := profile.Complete
	return n8nActionCompleteOutcomeMessage(scope.JobID, scope.AccountID, scope.N8NExecutionID, cfg.Action, cfg.Started, true, "", data), nil
}

func n8nCodexOAuthCompleteData(scope n8nActionScope, profile n8nCodexOAuthProfile, req *pb.N8NCodexOAuthCompleteRequest) *pb.ActivityCodexOAuthStepData {
	data := req.GetData()
	if data == nil {
		data = &pb.ActivityCodexOAuthStepData{}
	}
	if profile.IncludeAccountID {
		data.AccountId = strings.TrimSpace(scope.AccountID)
	}
	data.N8NExecutionId = strings.TrimSpace(scope.N8NExecutionID)
	data.AuthSecretKey = firstNonEmpty(data.GetAuthSecretKey(), req.GetAuthSecretKey())
	data.Driver = firstNonEmpty(data.GetDriver(), req.GetDriver())
	if req.AddPhoneConfirmed != nil {
		confirmed := req.GetAddPhoneConfirmed()
		data.AddPhoneConfirmed = &confirmed
	}
	return data
}

func n8nCodexOAuthFailureData(scope n8nActionScope, profile n8nCodexOAuthProfile, req *pb.N8NCodexOAuthFailRequest) *pb.ActivityCodexOAuthStepData {
	data := req.GetData()
	if data == nil {
		data = &pb.ActivityCodexOAuthStepData{}
	}
	if profile.IncludeAccountID {
		data.AccountId = strings.TrimSpace(scope.AccountID)
	}
	data.N8NExecutionId = strings.TrimSpace(scope.N8NExecutionID)
	data.ErrorMessage = firstNonEmpty(data.GetErrorMessage(), req.GetErrorMessage())
	if strings.TrimSpace(data.GetReason()) == "" {
		data.Reason = strings.TrimSpace(req.GetStep())
	}
	return data
}

func (s *Server) StartN8NCodexOAuthAddPhoneAccount(ctx context.Context, actionID string, req *pb.CodexOAuthAddPhoneRequest) (*pb.CodexOAuthAddPhoneResponse, string, error) {
	return startN8NCodexOAuthAction(ctx, s, n8nCodexOAuthStartConfig[*pb.CodexOAuthAddPhoneRequest, *pb.CodexOAuthAddPhoneResponse]{
		ActionID:  actionID,
		Response:  n8nCodexOAuthAddPhoneStartResponse,
		AccountID: codexOAuthRequestAccountID[*pb.CodexOAuthAddPhoneRequest],
		Params: func(req *pb.CodexOAuthAddPhoneRequest) (map[string]string, error) {
			return codexOAuthAddPhoneJobParams(req.GetAccountId(), req.GetLabel(), req.GetMaxReuseCount()), nil
		},
	}, req)
}

func (s *Server) CodexOAuthAcquirePhone(ctx context.Context, req *pb.CodexOAuthAcquirePhoneInput) (*pb.CodexOAuthPhoneLease, error) {
	return s.activities.CodexOAuthAcquirePhoneActivity(ctx, *req)
}

func (s *Server) CodexOAuthReleasePhone(ctx context.Context, req *pb.CodexOAuthReleasePhoneInput) (any, error) {
	if err := s.activities.CodexOAuthReleasePhoneActivity(ctx, *req); err != nil {
		return nil, err
	}
	return n8nActionSuccess(&pb.N8NActionSuccessResult{JobId: req.GetJobId(), ActivationId: req.GetActivationId()}), nil
}

func (s *Server) StartN8NCodexOAuthBatch(ctx context.Context, actionID string, req *pb.CodexOAuthBatchAddPhoneRequest) (*pb.CodexOAuthBatchAddPhoneResponse, error) {
	resp, _, err := startN8NCodexOAuthAction(ctx, s, n8nCodexOAuthStartConfig[*pb.CodexOAuthBatchAddPhoneRequest, *pb.CodexOAuthBatchAddPhoneResponse]{
		ActionID: actionID,
		Response: n8nCodexOAuthBatchAddPhoneStartResponse,
		Params: func(req *pb.CodexOAuthBatchAddPhoneRequest) (map[string]string, error) {
			accountIDs := compactAccountIDs(req.GetAccountIds())
			if len(accountIDs) == 0 {
				return nil, fmt.Errorf("account_ids is required")
			}
			return codexOAuthBatchAddPhoneJobParams(accountIDs, req.GetLabel(), req.GetMaxReuseCount()), nil
		},
	}, req)
	return resp, err
}

func (s *Server) CreateN8NCodexOAuthBatchAddPhoneChild(ctx context.Context, req *pb.N8NCodexOAuthBatchChildRequest) (any, error) {
	parentJobID := strings.TrimSpace(req.GetParentJobId())
	accountID := strings.TrimSpace(req.GetAccountId())
	if parentJobID == "" || accountID == "" {
		return nil, fmt.Errorf("parent_job_id and account_id are required")
	}
	n8nExecutionID := req.GetN8NExecutionId()
	if err := s.bindN8NExecution(ctx, parentJobID, n8nExecutionID); err != nil {
		return nil, err
	}
	profile, err := n8nCodexOAuthProfileForAction(contracts.ActionCodexOAuthAddPhone)
	if err != nil {
		return nil, err
	}
	label := req.GetLabel()
	maxReuseCount := req.GetMaxReuseCount()
	params := codexOAuthAddPhoneJobParams(accountID, label, maxReuseCount)
	params["parent_job_id"] = parentJobID
	childJobID := newN8NActionJobID()
	if err := s.createN8NActionJob(ctx, profile.Start, childJobID, accountID, "", params); err != nil {
		return nil, err
	}
	return n8nActionSuccess(&pb.N8NActionSuccessResult{
		ParentJobId:    parentJobID,
		JobId:          childJobID,
		AccountId:      accountID,
		Label:          label,
		MaxReuseCount:  maxReuseCount,
		N8NExecutionId: n8nExecutionID,
	}), nil
}

func startN8NCodexOAuthAction[Req any, Resp any](ctx context.Context, s *Server, cfg n8nCodexOAuthStartConfig[Req, Resp], req Req) (Resp, string, error) {
	profile, err := n8nCodexOAuthProfileForAction(cfg.ActionID)
	if err != nil {
		return n8nAccountStartResponse("", "", err, cfg.Response)
	}
	accountID, err := codexOAuthStartAccountID(req, cfg.AccountID)
	if err != nil {
		return n8nAccountStartResponse("", accountID, err, cfg.Response)
	}
	params, err := codexOAuthStartParams(req, cfg.Params)
	if err != nil {
		return n8nAccountStartResponse("", accountID, err, cfg.Response)
	}
	jobID := newN8NActionJobID()
	err = s.createN8NActionJob(ctx, profile.Start, jobID, accountID, "", params)
	return n8nAccountStartResponse(jobID, accountID, err, cfg.Response)
}

type codexOAuthAccountRequest interface {
	GetAccountId() string
}

func codexOAuthRequestAccountID[Req codexOAuthAccountRequest](req Req) (string, error) {
	return req.GetAccountId(), nil
}

func codexOAuthStartAccountID[Req any](req Req, accountID func(Req) (string, error)) (string, error) {
	if accountID == nil {
		return "", nil
	}
	value, err := accountID(req)
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("account_id is required")
	}
	return value, nil
}

func codexOAuthStartParams[Req any](req Req, params func(Req) (map[string]string, error)) (map[string]string, error) {
	if params == nil {
		return nil, nil
	}
	return params(req)
}
