package api

func goPayAppSMSOTPWaitRequest(req rawN8NActionRequest) n8nSMSOTPWaitRequest {
	return n8nSMSOTPWaitRequest{
		Action:           actionGoPayApp,
		JobID:            req.JobID,
		N8NExecutionID:   req.N8NExecutionID,
		Operation:        req.Operation,
		UserID:           goPayAppUserID(req.UserID),
		ActivationID:     req.ActivationID,
		StepName:         req.DataString("step_name"),
		TimeoutSeconds:   firstNonZeroInt32(req.OTPTimeoutSeconds, 300),
		OTPIssuedAfter:   req.OTPIssuedAfter,
		ResumeURL:        req.ResumeURL,
		OTPParam:         paymentOTPParam,
		SubmittedAtParam: paymentOTPSubmittedAtParam,
	}
}

func goPayPaymentSMSOTPWaitRequest(req rawN8NActionRequest) n8nSMSOTPWaitRequest {
	return n8nSMSOTPWaitRequest{
		Action:           actionGoPayPayment,
		JobID:            req.JobID,
		AccountID:        req.AccountID,
		N8NExecutionID:   req.N8NExecutionID,
		UserID:           goPayAppUserID(req.UserID),
		ActivationID:     req.ActivationID,
		StepName:         req.DataString("step_name"),
		TimeoutSeconds:   firstNonZeroInt32(req.OTPTimeoutSeconds, 300),
		OTPIssuedAfter:   req.OTPIssuedAfter,
		ResumeURL:        req.ResumeURL,
		OTPParam:         paymentOTPParam,
		SubmittedAtParam: paymentOTPSubmittedAtParam,
	}
}

func goPayPaymentRebindSMSOTPWaitRequest(req rawN8NActionRequest) n8nSMSOTPWaitRequest {
	out := goPayPaymentSMSOTPWaitRequest(req)
	out.Action = actionGoPayPaymentRebind
	out.Operation = req.Operation
	return out
}

func (r rawN8NActionRequest) DataString(key string) string {
	if r.Data == nil {
		return ""
	}
	value, _ := r.Data[key].(string)
	return value
}
