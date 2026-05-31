package activities

import (
	"context"
	"fmt"
	"orchestrator/internal/contracts"
	"orchestrator/internal/gptaccount"
	"strings"

	"orchestrator/pb"
)

func (s *Server) ProbePlusTrialAtomicActivity(ctx context.Context, input ProbePlusTrialActivityInput) (ProbePlusTrialActivityOutput, error) {
	var account *pb.Account
	accountID := strings.TrimSpace(input.GetAccountId())
	if accountID != "" {
		var err error
		account, err = s.getAccount(ctx, accountID)
		if err != nil {
			return ProbePlusTrialActivityOutput{}, err
		}
		if err := rejectUserAlreadyExistsAccount(account); err != nil {
			return ProbePlusTrialActivityOutput{}, err
		}
	}

	var output ProbePlusTrialActivityOutput
	var err error
	step := s.activityStep(ctx, input.GetJobId(), contracts.StepProbePlusTrial, false, true)
	_, err = step.run(func() (activityStepResult, error) {
		sessionToken := strings.TrimSpace(input.GetSessionToken())
		accessToken := strings.TrimSpace(input.GetAccessToken())
		if account != nil {
			if sessionToken == "" {
				sessionToken = s.cachedChatGPTSessionToken(ctx, gptaccount.ID(account))
			}
			if accessToken == "" {
				accessToken = s.cachedChatGPTAccessToken(ctx, gptaccount.ID(account))
			}
		}
		data := probePlusTrialStepData(accountID, sessionToken, accessToken)
		if sessionToken == "" && accessToken == "" {
			output.Data = data
			return data, fmt.Errorf("session_token or access_token is required")
		}

		credentialSessionToken := sessionToken
		if accessToken != "" {
			if sessionToken != "" {
				data.CredentialKind = "access_token_session_cookie"
			} else {
				data.CredentialKind = "access_token"
			}
		} else {
			data.CredentialKind = "session_token"
		}
		credential, callErr := s.paymentCredentialWithProxy(ctx, accountID, credentialSessionToken, accessToken, input.GetProxyUrl())
		if callErr != nil {
			output.Data = data
			return data, callErr
		}
		resp, callErr := s.paymentClient.ProbePlusTrial(ctx, &pb.ProbePlusTrialPaymentRequest{Credential: credential})
		applyPlusTrialProbeResponse(&output, data, resp)
		if callErr != nil {
			output.Data = data
			return data, callErr
		}
		if resp == nil {
			output.Data = data
			return data, fmt.Errorf("payment service returned empty probe response")
		}
		if !resp.GetSuccess() {
			msg := resp.GetErrorMessage()
			if msg == "" {
				msg = "plus trial probe failed"
			}
			output.Data = data
			return data, fmt.Errorf("%s", msg)
		}
		output.Data = data
		return data, nil
	})
	if err != nil {
		return output, err
	}
	return output, nil
}

func (s *Server) ProbeTierAtomicActivity(ctx context.Context, input ProbeTierActivityInput) (ProbeTierActivityOutput, error) {
	account, err := s.getAccount(ctx, input.GetAccountId())
	if err != nil {
		return ProbeTierActivityOutput{}, err
	}
	if err := rejectUserAlreadyExistsAccount(account); err != nil {
		return ProbeTierActivityOutput{}, err
	}

	var output ProbeTierActivityOutput
	step := s.activityStep(ctx, input.GetJobId(), contracts.StepProbeTier, false, true)
	_, err = step.run(func() (activityStepResult, error) {
		sessionToken := s.cachedChatGPTSessionToken(ctx, gptaccount.ID(account))
		accessToken := s.cachedChatGPTAccessToken(ctx, gptaccount.ID(account))
		data := probeTierStepData(gptaccount.ID(account), sessionToken, accessToken)
		if sessionToken == "" && accessToken == "" {
			output.Data = data
			return data, fmt.Errorf("session_token or access_token is required")
		}
		credentialSessionToken := sessionToken
		if accessToken != "" {
			credentialSessionToken = ""
			data.CredentialKind = "access_token"
		} else {
			data.CredentialKind = "session_token"
		}
		credential, callErr := s.paymentCredentialWithProxy(ctx, gptaccount.ID(account), credentialSessionToken, accessToken, input.GetProxyUrl())
		if callErr != nil {
			output.Data = data
			return data, callErr
		}
		resp, callErr := s.paymentClient.ProbeTier(ctx, &pb.ProbeTierPaymentRequest{Credential: credential})
		applyTierProbeResponse(&output, data, resp)
		if callErr != nil {
			output.Data = data
			return data, callErr
		}
		if resp == nil {
			output.Data = data
			return data, fmt.Errorf("payment service returned empty tier response")
		}
		if !resp.GetSuccess() {
			msg := resp.GetErrorMessage()
			if msg == "" {
				msg = "tier probe failed"
			}
			output.Data = data
			return data, fmt.Errorf("%s", msg)
		}
		output.Data = data
		return data, nil
	})
	if err != nil {
		return output, err
	}
	return output, nil
}
