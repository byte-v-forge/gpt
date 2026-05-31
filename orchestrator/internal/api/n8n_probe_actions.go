package api

import (
	"context"
	"fmt"
	"github.com/byte-v-forge/gpt/pkg/gptplugin"
	"strings"
	"time"

	"orchestrator/internal/accountauth"
	"orchestrator/internal/chatgptauth"
	"orchestrator/internal/contracts"
	"orchestrator/internal/gptaccount"
	"orchestrator/pb"
)

type chatGPTAccessTokenFetchResult struct {
	AccessToken     string
	ErrorMessage    string
	CredentialError bool
}

func (s *Server) StartN8NProbeAccount(ctx context.Context, accountID string) (*pb.ProbeAccountResponse, error) {
	return s.startN8NProbeJob(ctx, n8nProbeAccountProfile(), accountID)
}

func (s *Server) CheckN8NProbeAuthEdge(ctx context.Context, req *pb.N8NDynamicProxyCheckRequest) (any, error) {
	scope, err := s.bindN8NProbeScope(ctx, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId(), req.GetProxyUrl())
	if err != nil {
		return nil, err
	}
	profile := n8nProbeAccountProfile()
	target := profile.Proxy.normalized().AuthEdgeCheckTarget
	data := n8nAuthEdgeBaseData(scope.n8nActionScope, target)
	result := func(success bool) *pb.N8NDynamicProxyResult {
		return n8nDynamicProxyAuthEdgeResult(scope.n8nActionScope, scope.ProxyURL, success, data)
	}
	if scope.ProxyURL == "" {
		n8nAuthEdgeOutcome(data, false, target, "proxy url is required")
		return result(false), nil
	}
	if s == nil || s.runtimeSecrets == nil || s.paymentClient == nil {
		n8nAuthEdgeOutcome(data, false, target, "probe auth edge check is not configured")
		return result(false), nil
	}
	accessToken, _, err := accountauth.LoadChatGPTAccessToken(ctx, s.runtimeSecrets, scope.AccountID)
	if err != nil {
		return nil, err
	}
	if ttl, expiresAt, ok := chatgptauth.AccessTokenTTL(accessToken, time.Now()); ok && ttl > 0 {
		credential, err := s.paymentCredentialWithProxy(ctx, scope.AccountID, "", accessToken, scope.ProxyURL)
		if err != nil {
			return nil, err
		}
		probe, err := s.paymentClient.ProbeTier(ctx, &pb.ProbeTierPaymentRequest{Credential: credential})
		if err == nil && probe != nil && probe.GetSuccess() {
			n8nAuthEdgeOutcome(data, true, n8nAuthEdgeCheckTargetAccessToken, "")
			data.AccessTokenCached = protoBool(true)
			data.AccessTokenExpiresAtUnix = expiresAt
			data.AccessTokenTtlSeconds = int64(ttl.Seconds())
			return result(true), nil
		}
		message := "chatgpt access token edge check failed"
		if err != nil {
			message = err.Error()
		} else if probe != nil && strings.TrimSpace(probe.GetErrorMessage()) != "" {
			message = probe.GetErrorMessage()
		}
		n8nAuthEdgeOutcome(data, false, n8nAuthEdgeCheckTargetAccessToken, message)
		return result(false), nil
	}
	sessionToken, _, err := accountauth.LoadChatGPTSessionToken(ctx, s.runtimeSecrets, scope.AccountID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(sessionToken) == "" {
		n8nAuthEdgeOutcome(data, false, target, "session_token is required for checkout preflight")
		return result(false), nil
	}
	fetched, err := s.fetchAndCacheChatGPTAccessToken(ctx, scope.AccountID, sessionToken, scope.ProxyURL)
	if err != nil {
		return nil, err
	}
	if fetched.AccessToken == "" {
		message := firstNonEmpty(fetched.ErrorMessage, "chatgpt auth session edge check failed")
		n8nAuthEdgeOutcome(data, false, target, message)
		return result(false), nil
	}
	n8nAuthEdgeOutcome(data, true, target, "")
	data.AccessTokenCached = protoBool(true)
	return result(true), nil
}

func (s *Server) CheckN8NProbeToken(ctx context.Context, req *pb.N8NDynamicProxyCheckRequest) (any, error) {
	scope, err := s.bindN8NProbeScope(ctx, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId(), req.GetProxyUrl())
	if err != nil {
		return nil, err
	}
	accountResp, err := s.accountClient.GetAccount(ctx, &pb.GetAccountRequest{AccountId: scope.AccountID})
	if err != nil {
		return nil, err
	}
	account := accountResp.GetAccount()
	if account == nil || gptaccount.ID(account) == "" {
		return nil, fmt.Errorf("account not found")
	}
	data := &pb.N8NProbeTokenData{
		AccountId:      scope.AccountID,
		N8NExecutionId: scope.N8NExecutionID,
		ProxyUrl:       scope.ProxyURL,
		Account: probeAccountFacts(pb.AccountRef{
			AccountId:         gptaccount.ID(account),
			PlusTrialKnown:    account.PlusTrialEligible != nil,
			PlusTrialEligible: account.GetPlusTrialEligible(),
			PlusActive:        account.GetPlusActive(),
			Tier:              account.GetTier(),
		}),
	}
	result := func(tokenValid bool) *pb.N8NActionStepResult {
		data.TokenValid = protoBool(tokenValid)
		return scope.stepResultMessage(n8nProbeAccountProfile().TokenStep, tokenValid, data)
	}
	if s == nil || s.runtimeSecrets == nil {
		return nil, fmt.Errorf("runtime secret store is not configured")
	}
	cached, err := accountauth.LoadChatGPTAccessTokenSnapshot(ctx, s.runtimeSecrets, scope.AccountID)
	if err != nil {
		return nil, err
	}
	if cached.Present && strings.TrimSpace(cached.Value) != "" {
		data.AccessTokenCached = protoBool(true)
		data.RequiresLogin = protoBool(false)
		data.ExpiresAtUnix = cached.ExpiresAtUnix
		if ttl := time.Until(time.Unix(cached.ExpiresAtUnix, 0)); ttl > 0 {
			data.TtlSeconds = int64(ttl.Seconds())
		}
		return result(true), nil
	}
	sessionToken, _, err := accountauth.LoadChatGPTSessionToken(ctx, s.runtimeSecrets, scope.AccountID)
	if err != nil {
		return nil, err
	}
	if sessionToken == "" {
		data.RequiresLogin = protoBool(true)
		data.Reason = "session_token_missing"
		return result(false), nil
	}
	if s.paymentClient == nil {
		return nil, fmt.Errorf("payment service is not configured")
	}
	fetched, err := s.fetchAndCacheChatGPTAccessToken(ctx, scope.AccountID, sessionToken, scope.ProxyURL)
	if err != nil {
		if !fetched.CredentialError {
			return nil, err
		}
		data.RequiresLogin = protoBool(true)
		data.Reason = "account_fingerprint_missing"
		return result(false), nil
	}
	if fetched.AccessToken == "" {
		data.RequiresLogin = protoBool(true)
		data.Reason = "access_token_fetch_failed"
		return result(false), nil
	}
	accessToken := fetched.AccessToken
	ttl, expiresAt, ok := chatgptauth.AccessTokenTTL(accessToken, time.Now())
	if !ok {
		data.RequiresLogin = protoBool(true)
		data.ExpiresAtUnix = expiresAt
		data.Reason = "access_token_expired"
		return result(false), nil
	}
	data.RequiresLogin = protoBool(false)
	data.AccessTokenCached = protoBool(true)
	data.ExpiresAtUnix = expiresAt
	data.TtlSeconds = int64(ttl.Seconds())
	return result(true), nil
}

func (s *Server) fetchAndCacheChatGPTAccessToken(ctx context.Context, accountID string, sessionToken string, proxyURL string) (chatGPTAccessTokenFetchResult, error) {
	credential, err := s.paymentCredentialWithProxy(ctx, accountID, sessionToken, "", proxyURL)
	if err != nil {
		return chatGPTAccessTokenFetchResult{CredentialError: true}, err
	}
	fetched, err := s.paymentClient.FetchAccessToken(ctx, &pb.FetchAccessTokenPaymentRequest{Credential: credential})
	if err != nil {
		return chatGPTAccessTokenFetchResult{ErrorMessage: err.Error()}, nil
	}
	if fetched == nil {
		return chatGPTAccessTokenFetchResult{ErrorMessage: "chatgpt auth session edge check failed"}, nil
	}
	accessToken := strings.TrimSpace(fetched.GetAccessToken())
	if !fetched.GetSuccess() || accessToken == "" {
		return chatGPTAccessTokenFetchResult{
			ErrorMessage: firstNonEmpty(fetched.GetErrorMessage(), "chatgpt auth session edge check failed"),
		}, nil
	}
	if err := accountauth.SaveChatGPTAccessToken(ctx, s.runtimeSecrets, accountID, accessToken); err != nil {
		return chatGPTAccessTokenFetchResult{AccessToken: accessToken}, err
	}
	return chatGPTAccessTokenFetchResult{AccessToken: accessToken}, nil
}

func (s *Server) ProbeN8NPlusTrial(ctx context.Context, req *pb.N8NDynamicProxyCheckRequest) (any, error) {
	return s.runN8NProbeAtomicStep(ctx, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId(), req.GetProxyUrl(), contracts.StepProbePlusTrial, s.probePlusTrialAtomic)
}

func (s *Server) ProbeN8NTier(ctx context.Context, req *pb.N8NDynamicProxyCheckRequest) (any, error) {
	return s.runN8NProbeAtomicStep(ctx, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId(), req.GetProxyUrl(), contracts.StepProbeTier, s.probeTierAtomic)
}

func (s *Server) FailN8NProbeAccount(ctx context.Context, req *pb.N8NProbeFailRequest) (any, error) {
	scope, err := s.bindN8NProbeScope(ctx, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId(), "")
	if err != nil {
		return nil, err
	}
	return s.failStoredN8NActionMessage(ctx, n8nProbeAccountProfile().Failure, scope.JobID, scope.AccountID, scope.N8NExecutionID, req.GetStep(), req.GetErrorMessage(), n8nProbeFailureData(scope, req))
}

func (s *Server) CompleteN8NProbeAccount(ctx context.Context, req *pb.N8NProbeCompleteRequest) (any, error) {
	scope, err := s.bindN8NProbeScope(ctx, req.GetJobId(), req.GetAccountId(), req.GetN8NExecutionId(), "")
	if err != nil {
		return nil, err
	}
	plusTrial := nonNilProbePlusTrial(req.GetPlusTrial())
	tier := nonNilProbeTier(req.GetTier())
	resultData := &pb.N8NProbeCompleteData{
		AccountId:      scope.AccountID,
		N8NExecutionId: scope.N8NExecutionID,
		ProbePlusTrial: plusTrial,
		ProbeTier:      tier,
	}
	accountUpdated, err := s.applyN8NProbeAccountState(ctx, scope.AccountID, plusTrial, tier)
	if err != nil {
		resultData.AccountUpdateError = err.Error()
		return nil, err
	}
	resultData.AccountUpdated = protoBool(accountUpdated)
	if err := s.storeN8NActionSuccessMessage(ctx, scope.JobID, resultData); err != nil {
		return nil, err
	}
	success := plusTrial.GetSuccess() && tier.GetSuccess()
	message := ""
	if !success {
		message = "probe completed with unsuccessful result"
	}
	return n8nActionCompleteOutcomeMessage(scope.JobID, scope.AccountID, scope.N8NExecutionID, n8nProbeAccountProfile().Start.Action, true, success, message, resultData), nil
}

func nonNilProbePlusTrial(data *pb.ActivityProbePlusTrialStepData) *pb.ActivityProbePlusTrialStepData {
	if data == nil {
		return &pb.ActivityProbePlusTrialStepData{}
	}
	return data
}

func nonNilProbeTier(data *pb.ActivityProbeTierStepData) *pb.ActivityProbeTierStepData {
	if data == nil {
		return &pb.ActivityProbeTierStepData{}
	}
	return data
}

func n8nProbeFailureData(scope n8nProbeScope, req *pb.N8NProbeFailRequest) *pb.N8NProbeTokenData {
	data := req.GetData()
	if data == nil {
		data = &pb.N8NProbeTokenData{}
	}
	data.AccountId = strings.TrimSpace(scope.AccountID)
	data.N8NExecutionId = strings.TrimSpace(scope.N8NExecutionID)
	if strings.TrimSpace(data.GetReason()) == "" {
		data.Reason = firstNonEmpty(req.GetErrorMessage(), req.GetStep())
	}
	return data
}

func (s *Server) applyN8NProbeAccountState(ctx context.Context, accountID string, plusTrial *pb.ActivityProbePlusTrialStepData, tier *pb.ActivityProbeTierStepData) (bool, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return false, fmt.Errorf("account_id is required")
	}
	update := gptaccount.Patch(accountID)
	changed := false
	if value, ok := probeBool(plusTrial.PlusTrialEligible); ok {
		update.PlusTrialEligible = &value
		changed = true
	}
	if value, ok := probeBool(tier.PlusActive); ok {
		update.PlusActive = &value
		changed = true
	} else if value, ok := probeBool(plusTrial.PlusActive); ok {
		update.PlusActive = &value
		changed = true
	}
	if value := normalizeProbeTier(firstNonEmpty(
		tier.GetTier(),
		tier.GetTierProbe().GetTier(),
		plusTrial.GetTier(),
		plusTrial.GetPaymentProbe().GetTier(),
		plusTrial.GetPlanType(),
		plusTrial.GetPaymentProbe().GetPlanType(),
	)); value != "" {
		update.Tier = value
		changed = true
	}
	if update.PlusActive != nil && update.GetPlusActive() {
		gptaccount.SetStatus(update, gptplugin.AccountStatusActivated, "")
		changed = true
	}
	if !changed {
		return false, nil
	}
	if s.accountClient == nil {
		return false, fmt.Errorf("account service is not configured")
	}
	if _, err := s.accountClient.UpdateAccount(ctx, &pb.UpdateAccountRequest{Account: update}); err != nil {
		return false, err
	}
	return true, nil
}

func probeBool(value *bool) (bool, bool) {
	if value == nil {
		return false, false
	}
	return *value, true
}

func probeAccountFacts(account pb.AccountRef) *pb.N8NProbeAccountFacts {
	return &pb.N8NProbeAccountFacts{
		AccountId:         account.GetAccountId(),
		PlusTrialKnown:    account.GetPlusTrialKnown(),
		PlusTrialEligible: account.GetPlusTrialEligible(),
		Tier:              account.GetTier(),
		PlusActive:        account.GetPlusActive(),
	}
}

func normalizeProbeTier(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
