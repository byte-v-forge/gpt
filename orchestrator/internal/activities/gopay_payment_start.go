package activities

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/pb"
)

func (s *Server) startGoPayPayment(ctx context.Context, step activityStep, input GoPayActivityInput, account *pb.Account) (output GoPayPaymentStartOutput, err error) {
	sessionToken := strings.TrimSpace(input.GetSessionToken())
	if sessionToken == "" {
		sessionToken = s.cachedChatGPTSessionToken(ctx, account.GetAccountId())
	}
	accessToken := strings.TrimSpace(input.GetAccessToken())
	if accessToken == "" {
		accessToken = s.cachedChatGPTAccessToken(ctx, account.GetAccountId())
	}
	tokenization := strings.TrimSpace(input.GetTokenization())
	checkoutURL := strings.TrimSpace(input.GetCheckoutUrl())
	checkoutSessionID := strings.TrimSpace(input.GetCheckoutSessionId())
	otpChannel := normalizeGoPayOTPChannel(input.GetOtpChannel())
	useAccountToken := input.GetUseAccountToken()
	skipBalanceCheck := input.GetSkipAccountBalanceCheck()
	stateJSON := normalizeGoPayWorkflowStateJSON(input.GetStateJson())
	preparedFlowID := strings.TrimSpace(input.GetPreparedFlowId())
	requestedPhone := normalizeIndonesiaPhone(input.GetGopayPhone())
	accountPhone := ""
	qrisPayment := strings.EqualFold(tokenization, "qris")

	data := map[string]any{
		"account_id":             account.GetAccountId(),
		"session_token_present":  sessionToken != "",
		"access_token_present":   accessToken != "",
		"used_account_token":     useAccountToken,
		"tokenization":           tokenization,
		"otp_channel":            otpChannel,
		"checkout_url_present":   checkoutURL != "",
		"checkout_session_id":    checkoutSessionID,
		"prepared_flow_present":  preparedFlowID != "",
		"gopay_phone_present":    requestedPhone != "",
		"otp_value_recorded":     false,
		"payment_result_present": false,
	}
	output = GoPayPaymentStartOutput{
		FlowId:            preparedFlowID,
		OtpTimeoutSeconds: s.paymentOtpTimeout(),
		UseAccountToken:   useAccountToken,
		StateJson:         stateJSON,
	}
	defer func() {
		output.StateJson = stateJSON
		output.Data = protoData(data)
	}()
	if preparedFlowID == "" && !useAccountToken && sessionToken == "" && accessToken == "" {
		return output, fmt.Errorf("session_token or access_token is required")
	}

	step.progress("starting gopay payment", map[string]any{
		"use_account_token": useAccountToken,
		"tokenization":      tokenization,
		"prepared":          preparedFlowID != "",
	})
	accountToken := ""
	if useAccountToken {
		data["account_balance_check"] = map[string]any{"required_before_payment": !skipBalanceCheck}
		if skipBalanceCheck {
			data["account_balance_check"] = map[string]any{
				"required_before_payment": false,
				"skipped":                 true,
			}
			accountPhone = requestedPhone
			if accountPhone == "" {
				err := fmt.Errorf("gopay phone is required")
				data["gopay_phone"] = map[string]any{
					"present":       false,
					"error_message": err.Error(),
				}
				return output, err
			}
			data["account_token"] = map[string]any{
				"ready_check_skipped": true,
				"phone_present":       accountPhone != "",
				"phone":               accountPhone,
			}
		} else {
			step.progress("waiting for gopay min balance", nil)
			stateJSON, err = s.waitForGoPayMinBalance(ctx, step, stateJSON)
			if err != nil {
				data["account_balance_check"] = map[string]any{
					"required_before_payment": true,
					"ready":                   false,
					"error_message":           err.Error(),
				}
				return output, err
			}
			data["account_balance_check"] = map[string]any{
				"required_before_payment": true,
				"ready":                   true,
			}

			var phone string
			var nextStateJSON string
			accountToken, phone, nextStateJSON, err = s.readyGoPayAccountToken(ctx, stateJSON)
			stateJSON = nextStateJSON
			if err != nil {
				data["account_token"] = map[string]any{
					"ready":         false,
					"error_message": err.Error(),
				}
				return output, err
			}
			accountPhone = normalizeIndonesiaPhone(phone)
			if accountPhone == "" {
				accountPhone = requestedPhone
			}
			if accountPhone == "" {
				err := fmt.Errorf("account phone is required")
				data["account_token"] = map[string]any{
					"ready":         true,
					"phone_present": false,
					"error_message": err.Error(),
				}
				return output, err
			}
			if preparedFlowID == "" {
				sessionToken = ""
				accessToken = accountToken
				data["session_token_present"] = false
				data["access_token_present"] = accessToken != ""
			}
			data["account_token"] = map[string]any{
				"ready":         true,
				"phone_present": accountPhone != "",
				"phone":         accountPhone,
			}
		}
	}

	var started *pb.StartGoPayResponse
	startFresh := func() (*pb.StartGoPayResponse, error) {
		credential, err := s.paymentCredential(ctx, account.GetAccountId(), sessionToken, accessToken)
		if err != nil {
			return nil, err
		}
		return s.paymentClient.StartGoPay(ctx, &pb.StartGoPayRequest{
			Credential:        credential,
			UseAccountToken:   useAccountToken,
			Tokenization:      tokenization,
			CheckoutUrl:       checkoutURL,
			CheckoutSessionId: checkoutSessionID,
			GopayPhone:        accountPhone,
			OtpChannel:        otpChannel,
			GopayCountryCode:  input.GetCountryCode(),
		})
	}
	if preparedFlowID != "" {
		if accountPhone == "" {
			accountPhone = requestedPhone
		}
		if accountPhone == "" && !qrisPayment {
			err := fmt.Errorf("gopay phone is required for prepared payment")
			data["gopay_phone"] = map[string]any{
				"present":       false,
				"error_message": err.Error(),
			}
			return output, err
		}
		data["gopay_phone"] = map[string]any{
			"present": accountPhone != "",
			"phone":   accountPhone,
		}
		started, err = s.paymentClient.StartPreparedGoPay(ctx, &pb.StartPreparedGoPayRequest{
			FlowId:           preparedFlowID,
			GopayPhone:       accountPhone,
			OtpChannel:       otpChannel,
			GopayCountryCode: input.GetCountryCode(),
		})
		if isStalePreparedPaymentFlow(started, err) && useAccountToken && accountToken != "" {
			data["prepared_flow_stale"] = paymentStartData(started)
			preparedFlowID = ""
			output.FlowId = ""
			sessionToken = ""
			accessToken = accountToken
			data["prepared_flow_present"] = false
			data["session_token_present"] = false
			data["access_token_present"] = true
			step.progress("prepared gopay payment flow missing; starting fresh payment", map[string]any{
				"use_account_token": true,
			})
			started, err = startFresh()
		}
	} else {
		started, err = startFresh()
	}
	data["payment_start"] = paymentStartData(started)
	if started != nil {
		output.FlowId = started.GetFlowId()
		output.IssuedAfterUnix = started.GetIssuedAfterUnix()
		output.OtpRequired = started.GetOtpRequired()
	}
	step.progress("gopay payment started", map[string]any{
		"success":           started != nil && started.GetSuccess(),
		"issued_after_unix": output.GetIssuedAfterUnix(),
		"otp_required":      output.GetOtpRequired(),
	})
	if err != nil {
		return output, err
	}
	if started == nil {
		return output, fmt.Errorf("payment start returned empty response")
	}
	if !started.GetSuccess() {
		return output, fmt.Errorf("payment start failed: %s", started.GetErrorMessage())
	}
	if output.GetFlowId() == "" {
		return output, fmt.Errorf("payment start returned empty flow_id")
	}
	return output, nil
}
