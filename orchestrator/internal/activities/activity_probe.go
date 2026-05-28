package activities

import (
	"context"
	"fmt"
	"orchestrator/pb"
	"strings"
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
	step := s.activityStep(ctx, input.GetJobId(), stepProbePlusTrial, false, true)
	_, err = step.run(func() (any, error) {
		sessionToken := strings.TrimSpace(input.GetSessionToken())
		accessToken := strings.TrimSpace(input.GetAccessToken())
		if account != nil {
			if sessionToken == "" {
				sessionToken = s.cachedChatGPTSessionToken(ctx, account.GetAccountId())
			}
			if accessToken == "" {
				accessToken = s.cachedChatGPTAccessToken(ctx, account.GetAccountId())
			}
		}
		data := map[string]any{
			"account_id":            accountID,
			"session_token_present": sessionToken != "",
			"access_token_present":  accessToken != "",
		}
		if sessionToken == "" && accessToken == "" {
			output.Data = protoData(data)
			return data, fmt.Errorf("session_token or access_token is required")
		}

		credential, callErr := s.paymentCredentialWithProxy(ctx, accountID, sessionToken, accessToken, input.GetProxyUrl())
		if callErr != nil {
			output.Data = protoData(data)
			return data, callErr
		}
		resp, callErr := s.paymentClient.ProbePlusTrial(ctx, &pb.ProbePlusTrialPaymentRequest{Credential: credential})
		data["payment_probe"] = plusTrialProbeData(resp)
		if resp != nil {
			output.Success = resp.GetSuccess()
			output.Checked = resp.GetChecked()
			output.PlusTrialEligible = resp.GetPlusTrialEligible()
			output.PlusActive = resp.GetPlusActive()
			output.Amount = resp.GetAmount()
			output.Currency = resp.GetCurrency()
			output.Source = resp.GetSource()
			output.PlanType = resp.GetPlanType()
			output.CheckoutUrl = resp.GetCheckoutUrl()
			output.CheckoutSessionId = resp.GetCheckoutSessionId()
			output.ErrorMessage = resp.GetErrorMessage()
			data["success"] = resp.GetSuccess()
			data["checked"] = resp.GetChecked()
			data["plus_trial_eligible"] = resp.GetPlusTrialEligible()
			data["plus_active"] = resp.GetPlusActive()
			data["plan_type"] = resp.GetPlanType()
			data["amount"] = resp.GetAmount()
			data["currency"] = resp.GetCurrency()
			data["source"] = resp.GetSource()
			data["checkout_url"] = resp.GetCheckoutUrl()
			data["checkout_session_id"] = resp.GetCheckoutSessionId()
			data["error_message"] = resp.GetErrorMessage()
		}
		if callErr != nil {
			output.Data = protoData(data)
			return data, callErr
		}
		if resp == nil {
			output.Data = protoData(data)
			return data, fmt.Errorf("payment service returned empty probe response")
		}
		if !resp.GetSuccess() {
			msg := resp.GetErrorMessage()
			if msg == "" {
				msg = "plus trial probe failed"
			}
			output.Data = protoData(data)
			return data, fmt.Errorf("%s", msg)
		}
		if resp.GetChecked() {
			tier := normalizeTier(resp.GetPlanType())
			if tier == "" && !resp.GetPlusActive() {
				tier = "free"
			}
			if accountID != "" {
				update := &pb.Account{
					AccountId:         accountID,
					PlusTrialEligible: boolPtr(resp.GetPlusTrialEligible()),
					PlusActive:        boolPtr(resp.GetPlusActive()),
					Tier:              tier,
				}
				if resp.GetPlusActive() {
					update.Status = accountStatusActivated
					update.ErrorMessage = ""
				}
				if updateErr := s.updateAccount(ctx, update); updateErr != nil {
					data["account_update_error"] = updateErr.Error()
					output.Data = protoData(data)
					return data, updateErr
				}
				data["account_updated"] = true
			}
		}
		output.Data = protoData(data)
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
	step := s.activityStep(ctx, input.GetJobId(), stepProbeTier, false, true)
	_, err = step.run(func() (any, error) {
		sessionToken := s.cachedChatGPTSessionToken(ctx, account.GetAccountId())
		accessToken := s.cachedChatGPTAccessToken(ctx, account.GetAccountId())
		data := map[string]any{
			"account_id":            account.GetAccountId(),
			"session_token_present": sessionToken != "",
			"access_token_present":  accessToken != "",
		}
		if sessionToken == "" && accessToken == "" {
			output.Data = protoData(data)
			return data, fmt.Errorf("session_token or access_token is required")
		}
		credential, callErr := s.paymentCredentialWithProxy(ctx, account.GetAccountId(), sessionToken, accessToken, input.GetProxyUrl())
		if callErr != nil {
			output.Data = protoData(data)
			return data, callErr
		}
		resp, callErr := s.paymentClient.ProbeTier(ctx, &pb.ProbeTierPaymentRequest{Credential: credential})
		data["tier_probe"] = tierProbeData(resp)
		if resp != nil {
			output.Success = resp.GetSuccess()
			output.Checked = resp.GetChecked()
			output.Tier = normalizeTier(resp.GetTier())
			output.PlusActive = resp.GetPlusActive()
			output.Source = resp.GetSource()
			output.ErrorMessage = resp.GetErrorMessage()
			data["success"] = resp.GetSuccess()
			data["checked"] = resp.GetChecked()
			data["tier"] = output.Tier
			data["plus_active"] = resp.GetPlusActive()
			data["source"] = resp.GetSource()
			data["error_message"] = resp.GetErrorMessage()
		}
		if callErr != nil {
			output.Data = protoData(data)
			return data, callErr
		}
		if resp == nil {
			output.Data = protoData(data)
			return data, fmt.Errorf("payment service returned empty tier response")
		}
		if !resp.GetSuccess() {
			msg := resp.GetErrorMessage()
			if msg == "" {
				msg = "tier probe failed"
			}
			output.Data = protoData(data)
			return data, fmt.Errorf("%s", msg)
		}
		if resp.GetChecked() {
			update := &pb.Account{
				AccountId:  input.GetAccountId(),
				Tier:       output.Tier,
				PlusActive: boolPtr(resp.GetPlusActive()),
			}
			if resp.GetPlusActive() {
				update.Status = accountStatusActivated
				update.ErrorMessage = ""
			}
			if updateErr := s.updateAccount(ctx, update); updateErr != nil {
				data["account_update_error"] = updateErr.Error()
				output.Data = protoData(data)
				return data, updateErr
			}
			data["account_updated"] = true
		}
		output.Data = protoData(data)
		return data, nil
	})
	if err != nil {
		return output, err
	}
	return output, nil
}
