package api

func goPayAppPaymentOTPWaitRequest(req rawN8NActionRequest) n8nPaymentOTPWaitRequest {
	return n8nPaymentOTPWaitRequest{
		Action:           actionGoPayApp,
		JobID:            req.JobID,
		N8NExecutionID:   req.N8NExecutionID,
		Operation:        req.Operation,
		UserID:           goPayAppUserID(req.UserID),
		Source:           firstNonEmpty(req.Source, req.UserID),
		Purpose:          req.Purpose,
		StepName:         req.DataString("step_name"),
		TimeoutSeconds:   firstNonZeroInt32(req.OTPTimeoutSeconds, 300),
		OTPIssuedAfter:   req.OTPIssuedAfter,
		ResumeURL:        req.ResumeURL,
		OTPParam:         paymentOTPParam,
		SubmittedAtParam: paymentOTPSubmittedAtParam,
	}
}

func goPayPaymentPaymentOTPWaitRequest(req rawN8NActionRequest) n8nPaymentOTPWaitRequest {
	return n8nPaymentOTPWaitRequest{
		Action:           actionGoPayPayment,
		JobID:            req.JobID,
		AccountID:        req.AccountID,
		N8NExecutionID:   req.N8NExecutionID,
		UserID:           goPayAppUserID(req.UserID),
		Source:           firstNonEmpty(req.Source, req.UserID),
		Purpose:          req.Purpose,
		StepName:         req.DataString("step_name"),
		TimeoutSeconds:   firstNonZeroInt32(req.OTPTimeoutSeconds, 300),
		OTPIssuedAfter:   req.OTPIssuedAfter,
		ResumeURL:        req.ResumeURL,
		OTPParam:         paymentOTPParam,
		SubmittedAtParam: paymentOTPSubmittedAtParam,
	}
}

func goPayPaymentRebindPaymentOTPWaitRequest(req rawN8NActionRequest) n8nPaymentOTPWaitRequest {
	out := goPayPaymentPaymentOTPWaitRequest(req)
	out.Action = actionGoPayPaymentRebind
	out.Operation = req.Operation
	return out
}

func goPayWAPaymentOTPWaitRequest(req rawN8NActionRequest) n8nPaymentOTPWaitRequest {
	out := goPayPaymentPaymentOTPWaitRequest(req)
	out.Action = actionGoPayWAPayment
	return out
}

func paymentOTPReceiveRequest(req rawN8NActionRequest) n8nPaymentOTPReceiveRequest {
	return n8nPaymentOTPReceiveRequest{Source: req.Source, Purpose: req.Purpose, OTP: req.OTP, OTPSource: req.OTPSource, ReceivedAtUnix: req.OTPReceivedAt}
}
