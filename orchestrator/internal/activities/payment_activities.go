package activities

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"orchestrator/pb"
)

func (s *Server) GoPayPaymentPrepareActivity(ctx context.Context, input GoPayActivityInput) (GoPayPaymentPrepareOutput, error) {
	output := GoPayPaymentPrepareOutput{}
	account, err := s.getAccount(ctx, input.GetAccountId())
	if err != nil {
		return output, err
	}
	if err := accountEligibleForActivation(account); err != nil {
		return output, err
	}
	step, err := s.startActivityStep(ctx, input.GetJobId(), stepGoPayPaymentPrepare, false, true)
	if err != nil {
		return output, err
	}

	output, err = s.prepareGoPayPayment(ctx, step, input, account)
	if err != nil {
		return output, step.complete(protoDataMap(output.GetData()), err)
	}
	return output, step.complete(protoDataMap(output.GetData()), nil)
}

func (s *Server) GoPayPaymentStartActivity(ctx context.Context, input GoPayActivityInput) (GoPayPaymentStartOutput, error) {
	output := GoPayPaymentStartOutput{}
	account, err := s.getAccount(ctx, input.GetAccountId())
	if err != nil {
		return output, err
	}
	if err := accountEligibleForActivation(account); err != nil {
		return output, err
	}
	step, err := s.startActivityStep(ctx, input.GetJobId(), stepGoPayPayment, false, true)
	if err != nil {
		return output, err
	}

	output, err = s.startGoPayPayment(ctx, step, input, account)
	if err != nil {
		return output, s.completeGoPayPaymentStep(ctx, input.GetJobId(), input.GetAccountId(), protoDataMap(output.GetData()), err)
	}
	step.update(protoDataMap(output.GetData()))
	return output, nil
}

func (s *Server) GoPayPaymentCompleteActivity(ctx context.Context, input GoPayPaymentCompleteInput) (GoPayActivityOutput, error) {
	step := s.activityStep(ctx, input.GetJobId(), stepGoPayPayment, false, true)
	stateJSON := normalizeGoPayWorkflowStateJSON(input.GetStateJson())
	data := protoDataMap(input.GetData())
	if data == nil {
		data = map[string]any{}
	}
	data["payment_otp"] = map[string]any{
		"timeout_seconds":    s.paymentOtpTimeout(),
		"issued_after_unix":  input.GetOtpIssuedAfterUnix(),
		"found":              input.GetOtpSource() != "not_required",
		"source":             input.GetOtpSource(),
		"manual_allowed":     true,
		"otp_value_recorded": false,
	}
	otp := ""
	if input.GetOtpSource() == "not_required" {
		step.progress("payment otp not required", nil)
	} else {
		step.progress("payment otp received", map[string]any{
			"source": input.GetOtpSource(),
		})
		var err error
		otp, err = s.consumeStoredOTP(ctx, input.GetJobId(), input.GetOtpParam(), input.GetSubmittedAtParam(), input.GetOtpIssuedAfterUnix())
		if err != nil {
			return GoPayActivityOutput{Data: protoData(data), StateJson: stateJSON}, s.completeGoPayPaymentStep(ctx, input.GetJobId(), input.GetAccountId(), data, err)
		}
	}

	result, stateJSON, err := s.completeGoPayPayment(ctx, step, stateJSON, input.GetFlowId(), otp, input.GetUseAccountToken(), data)
	if err != nil {
		return GoPayActivityOutput{Data: protoData(data), StateJson: stateJSON}, s.completeGoPayPaymentStep(ctx, input.GetJobId(), input.GetAccountId(), data, err)
	}

	output := GoPayActivityOutput{
		ChargeRef:         result.GetChargeRef(),
		SnapToken:         result.GetSnapToken(),
		PlusTrialEligible: true,
		PlusTrialChecked:  true,
		PlusActive:        true,
		Data:              protoData(data),
		StateJson:         stateJSON,
	}
	return output, step.complete(data, nil)
}

func (s *Server) GoPayPaymentOTPResendActivity(ctx context.Context, input GoPayPaymentOTPResendInput) (GoPayPaymentOTPResendOutput, error) {
	output := GoPayPaymentOTPResendOutput{FlowId: strings.TrimSpace(input.GetFlowId())}
	step := s.activityStep(ctx, input.GetJobId(), stepGoPayPayment, false, true)
	data := protoDataMap(input.GetData())
	if data == nil {
		data = map[string]any{}
	}
	resends, _ := data["payment_otp_resends"].([]any)
	if output.GetFlowId() == "" {
		err := fmt.Errorf("flow_id is required")
		data["payment_otp_resend"] = map[string]any{"success": false, "error_message": err.Error()}
		output.Data = protoData(data)
		step.update(data)
		return output, err
	}

	step.progress("resending gopay payment otp", map[string]any{
		"flow_id_present": true,
		"resend_attempt":  len(resends) + 1,
	})
	resp, err := s.paymentClient.ResendGoPayOTP(ctx, &pb.ResendGoPayOTPRequest{FlowId: output.GetFlowId()})
	item := paymentOTPResendData(resp)
	resends = append(resends, item)
	data["payment_otp_resend"] = item
	data["payment_otp_resends"] = resends
	if resp != nil {
		output.Success = resp.GetSuccess()
		output.FlowId = resp.GetFlowId()
		output.IssuedAfterUnix = resp.GetIssuedAfterUnix()
	}
	output.Data = protoData(data)
	step.update(data)
	if err != nil {
		return output, err
	}
	if resp == nil {
		return output, fmt.Errorf("payment otp resend returned empty response")
	}
	if !resp.GetSuccess() {
		return output, fmt.Errorf("payment otp resend failed: %s", resp.GetErrorMessage())
	}
	step.progress("gopay payment otp resent", map[string]any{
		"issued_after_unix": output.GetIssuedAfterUnix(),
	})
	return output, nil
}

func (s *Server) GoPayPaymentCancelActivity(ctx context.Context, input GoPayPaymentCancelInput) error {
	if strings.TrimSpace(input.GetFlowId()) == "" {
		return nil
	}
	resp, err := s.paymentClient.CancelGoPay(ctx, &pb.CancelGoPayRequest{FlowId: input.GetFlowId()})
	if err != nil {
		return err
	}
	if resp != nil && !resp.GetSuccess() {
		return fmt.Errorf("payment cancel failed: %s", resp.GetErrorMessage())
	}
	return nil
}

func (s *Server) prepareGoPayPayment(ctx context.Context, step activityStep, input GoPayActivityInput, account *pb.Account) (output GoPayPaymentPrepareOutput, err error) {
	sessionToken := strings.TrimSpace(input.GetSessionToken())
	if sessionToken == "" {
		sessionToken = strings.TrimSpace(account.GetSessionToken())
	}
	accessToken := strings.TrimSpace(input.GetAccessToken())
	if accessToken == "" {
		accessToken = strings.TrimSpace(account.GetAccessToken())
	}
	tokenization := strings.TrimSpace(input.GetTokenization())
	checkoutURL := strings.TrimSpace(input.GetCheckoutUrl())
	checkoutSessionID := strings.TrimSpace(input.GetCheckoutSessionId())
	gopayPhone := normalizeIndonesiaPhone(input.GetGopayPhone())
	stateJSON := normalizeGoPayWorkflowStateJSON(input.GetStateJson())

	data := map[string]any{
		"account_id":             account.GetAccountId(),
		"session_token_present":  sessionToken != "",
		"access_token_present":   accessToken != "",
		"tokenization":           tokenization,
		"checkout_url_present":   checkoutURL != "",
		"checkout_session_id":    checkoutSessionID,
		"gopay_phone_present":    gopayPhone != "",
		"prepared_flow_present":  false,
		"payment_prepare_called": false,
	}
	output = GoPayPaymentPrepareOutput{
		UseAccountToken: false,
		StateJson:       stateJSON,
	}
	defer func() {
		output.StateJson = stateJSON
		output.Data = protoData(data)
	}()
	if sessionToken == "" && accessToken == "" {
		return output, fmt.Errorf("session_token or access_token is required")
	}

	step.progress("preparing gopay payment before user link", map[string]any{
		"tokenization":         tokenization,
		"checkout_url_present": checkoutURL != "",
		"checkout_session_id":  checkoutSessionID,
		"gopay_phone_present":  gopayPhone != "",
	})
	prepared, err := s.paymentClient.PrepareGoPay(ctx, &pb.PrepareGoPayRequest{
		SessionToken:      sessionToken,
		AccessToken:       accessToken,
		Tokenization:      tokenization,
		CheckoutUrl:       checkoutURL,
		CheckoutSessionId: checkoutSessionID,
		GopayPhone:        gopayPhone,
	})
	data["payment_prepare_called"] = true
	data["payment_prepare"] = paymentPrepareData(prepared)
	if prepared != nil {
		output.FlowId = prepared.GetFlowId()
		output.SnapToken = prepared.GetSnapToken()
		output.CheckoutUrl = prepared.GetCheckoutUrl()
		output.CheckoutSessionId = prepared.GetCheckoutSessionId()
		data["prepared_flow_present"] = output.GetFlowId() != ""
	}
	step.progress("gopay payment prepared", map[string]any{
		"success":            prepared != nil && prepared.GetSuccess(),
		"flow_id_present":    output.GetFlowId() != "",
		"snap_token_present": output.GetSnapToken() != "",
	})
	if err != nil {
		return output, err
	}
	if prepared == nil {
		return output, fmt.Errorf("payment prepare returned empty response")
	}
	if !prepared.GetSuccess() {
		return output, fmt.Errorf("payment prepare failed: %s", prepared.GetErrorMessage())
	}
	if output.GetFlowId() == "" {
		return output, fmt.Errorf("payment prepare returned empty flow_id")
	}
	return output, nil
}

func (s *Server) startGoPayPayment(ctx context.Context, step activityStep, input GoPayActivityInput, account *pb.Account) (output GoPayPaymentStartOutput, err error) {
	sessionToken := strings.TrimSpace(input.GetSessionToken())
	if sessionToken == "" {
		sessionToken = strings.TrimSpace(account.GetSessionToken())
	}
	accessToken := strings.TrimSpace(input.GetAccessToken())
	if accessToken == "" {
		accessToken = strings.TrimSpace(account.GetAccessToken())
	}
	tokenization := strings.TrimSpace(input.GetTokenization())
	checkoutURL := strings.TrimSpace(input.GetCheckoutUrl())
	checkoutSessionID := strings.TrimSpace(input.GetCheckoutSessionId())
	otpChannel := normalizeGoPayOTPChannel(input.GetOtpChannel())
	useAccountToken := input.GetUseAccountToken()
	stateJSON := normalizeGoPayWorkflowStateJSON(input.GetStateJson())
	preparedFlowID := strings.TrimSpace(input.GetPreparedFlowId())
	requestedPhone := normalizeIndonesiaPhone(input.GetGopayPhone())
	accountPhone := ""

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
		data["account_balance_check"] = map[string]any{"required_before_payment": true}
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
			accountPhone = configuredGoPayPhone()
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

	var started *pb.StartGoPayResponse
	startFresh := func() (*pb.StartGoPayResponse, error) {
		return s.paymentClient.StartGoPay(ctx, &pb.StartGoPayRequest{
			SessionToken:      sessionToken,
			AccessToken:       accessToken,
			UseAccountToken:   useAccountToken,
			Tokenization:      tokenization,
			CheckoutUrl:       checkoutURL,
			CheckoutSessionId: checkoutSessionID,
			GopayPhone:        accountPhone,
			OtpChannel:        otpChannel,
		})
	}
	if preparedFlowID != "" {
		if accountPhone == "" {
			accountPhone = requestedPhone
		}
		if accountPhone == "" {
			accountPhone = configuredGoPayPhone()
		}
		if accountPhone == "" {
			err := fmt.Errorf("gopay phone is required for prepared payment")
			data["gopay_phone"] = map[string]any{
				"present":       false,
				"error_message": err.Error(),
			}
			return output, err
		}
		data["gopay_phone"] = map[string]any{
			"present": true,
			"phone":   accountPhone,
		}
		started, err = s.paymentClient.StartPreparedGoPay(ctx, &pb.StartPreparedGoPayRequest{
			FlowId:     preparedFlowID,
			GopayPhone: accountPhone,
			OtpChannel: otpChannel,
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

func (s *Server) completeGoPayPayment(ctx context.Context, step activityStep, stateJSON, flowID, otp string, useAccountToken bool, data map[string]any) (*pb.GoPayResponse, string, error) {
	stateJSON = normalizeGoPayWorkflowStateJSON(stateJSON)
	completed := false
	defer func() {
		if flowID != "" && !completed {
			cancelCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cancelResp, cancelErr := s.paymentClient.CancelGoPay(cancelCtx, &pb.CancelGoPayRequest{FlowId: flowID})
			data["cleanup"] = cleanupDataFromPayment(cancelResp, cancelErr)
		}
	}()

	result, err := s.paymentClient.CompleteGoPay(ctx, &pb.CompleteGoPayRequest{FlowId: flowID, Otp: otp})
	data["payment_complete"] = paymentResultData(result)
	data["payment_result_present"] = result != nil
	step.progress("gopay payment complete called", map[string]any{
		"success":                      result != nil && result.GetSuccess(),
		"awaiting_manual_confirmation": result != nil && result.GetAwaitingManualConfirmation(),
	})
	if err != nil {
		return nil, stateJSON, err
	}
	if result == nil {
		return nil, stateJSON, fmt.Errorf("payment complete returned empty response")
	}
	if !result.GetSuccess() {
		return nil, stateJSON, fmt.Errorf("payment complete failed: %s", result.GetErrorMessage())
	}
	if result.GetAwaitingManualConfirmation() {
		data["manual_payment_confirmation"] = map[string]any{
			"required":      true,
			"auto_expected": true,
			"confirmed":     false,
		}
		if !useAccountToken {
			return nil, stateJSON, fmt.Errorf("payment requires manual confirmation; QR autopay did not settle automatically")
		}

		replayResp, nextStateJSON, replayErr := s.replayGoPayPaymentLink(ctx, stateJSON, result)
		stateJSON = nextStateJSON
		data["gopay_link_payment"] = replayLinkPaymentData(replayResp, replayErr)
		step.progress("gopay link payment replayed", map[string]any{
			"success": replayResp != nil && replayResp.GetSuccess(),
		})
		if replayErr != nil {
			return nil, stateJSON, replayErr
		}

		result, err = s.paymentClient.ConfirmGoPayPayment(ctx, &pb.ConfirmGoPayPaymentRequest{FlowId: flowID})
		data["payment_confirm"] = paymentResultData(result)
		data["payment_result_present"] = result != nil
		step.progress("gopay payment confirmed", map[string]any{
			"success": result != nil && result.GetSuccess(),
		})
		if err != nil {
			return nil, stateJSON, err
		}
		if result == nil {
			return nil, stateJSON, fmt.Errorf("payment confirm returned empty response")
		}
		if !result.GetSuccess() {
			return nil, stateJSON, fmt.Errorf("payment confirm failed: %s", result.GetErrorMessage())
		}
		data["manual_payment_confirmation"] = map[string]any{
			"required":      true,
			"auto_expected": true,
			"confirmed":     true,
		}
	}
	completed = true

	if useAccountToken {
		nextStateJSON, unlinkErr := s.unlinkGoPayAccountToken(ctx, stateJSON)
		stateJSON = nextStateJSON
		if unlinkErr != nil {
			log.Printf("[gopay-app] Unlink after payment failed: %v", unlinkErr)
			data["gopay_unlink"] = cleanupData(false, unlinkErr.Error(), unlinkErr)
		} else {
			data["gopay_unlink"] = cleanupData(true, "", nil)
		}
	}
	return result, stateJSON, nil
}

func (s *Server) completeGoPayPaymentStep(ctx context.Context, jobID, accountID string, data map[string]any, err error) error {
	if err != nil && isFreeTrialIneligibleError(err) {
		if updateErr := s.updateAccount(ctx, &pb.Account{
			AccountId:         accountID,
			PlusTrialEligible: boolPtr(false),
		}); updateErr != nil {
			err = fmt.Errorf("%w; additionally failed to mark plus trial ineligible: %v", err, updateErr)
		}
	}
	return s.completeActivityStep(ctx, jobID, stepGoPayPayment, false, true, data, err)
}

func isStalePreparedPaymentFlow(resp *pb.StartGoPayResponse, err error) bool {
	message := ""
	if err != nil {
		message = err.Error()
	}
	if resp != nil && resp.GetErrorMessage() != "" {
		if message != "" {
			message += " "
		}
		message += resp.GetErrorMessage()
	}
	message = strings.ToLower(message)
	return strings.Contains(message, "prepared payment flow not found") ||
		strings.Contains(message, "payment flow not found")
}
