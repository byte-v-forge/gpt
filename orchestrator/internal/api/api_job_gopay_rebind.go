package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"orchestrator/internal/jobstatus"
	"orchestrator/internal/otpwait"
	"orchestrator/pb"
)

const (
	goPayLocalSource               = "local"
	goPayPaymentRebindPinSecretKey = "gopay_payment_rebind_pin:"
)

type goPayAppActionOptions struct {
	Phone           string
	OTPChannel      string
	SMSActivationID string
	Source          string
	ResetState      bool
	StateJSON       string
	Pin             string
	CountryCode     string
	SkipPhoneProbe  bool
}

func (s *Server) runGoPayPaymentRebindAction(ctx context.Context, jobID string, params map[string]string) error {
	pin, err := s.runtimeSecretValue(ctx, params["pin_secret_key"])
	if err != nil {
		return s.markActionFailed(ctx, jobID, "load_runtime_secret", jobstatus.FailedFinal, false, false, err, nil)
	}
	defer s.deleteRuntimeSecretValue(context.Background(), params["pin_secret_key"])

	source, err := s.activities.GoPayPaymentRebindSourceActivity(ctx, pb.GoPayPaymentRebindSourceInput{
		JobId:       jobID,
		SourceJobId: params["source_job_id"],
		AccountId:   params["account_id"],
		UserId:      params["user_id"],
	})
	combined := map[string]any{
		"source_job_id": params["source_job_id"],
		"rebind_source": structMap(source.GetData()),
	}
	if err != nil {
		return s.markActionFailed(ctx, jobID, "resolve_rebind_source", jobstatus.FailedRetryable, false, true, err, combined)
	}
	userID := strings.TrimSpace(source.GetUserId())
	if userID == "" {
		userID = goPayLocalSource
	}
	combined["account_id"] = source.GetAccountId()
	combined["user_id"] = userID
	combined["wa_phone_present"] = strings.TrimSpace(source.GetWaPhone()) != ""

	stored, err := s.activities.GoPayAppLoadStateActivity(ctx, pb.GoPayAppStateActivityInput{JobId: jobID, UserId: userID, Reason: "payment_rebind_retry"})
	mergeActionData(combined, "load_gopay_state", structMap(stored.GetData()))
	if err != nil {
		return s.markActionFailed(ctx, jobID, "load_gopay_state", jobstatus.FailedRetryable, false, true, err, combined)
	}
	stateJSON := strings.TrimSpace(stored.GetStateJson())
	if stateJSON == "" {
		stateJSON = "{}"
	}

	auth, err := s.runGoPayAppAuth(ctx, jobID, goPayAppActionOptions{Phone: source.GetWaPhone(), OTPChannel: "wa", Source: userID, StateJSON: stateJSON, Pin: pin, CountryCode: params["country_code"]})
	mergeActionData(combined, "login", structMap(auth.GetData()))
	if nextStateJSON := strings.TrimSpace(auth.GetStateJson()); nextStateJSON != "" {
		stateJSON = nextStateJSON
	}
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayAppLogin, jobstatus.FailedRetryable, false, true, err, combined)
	}
	if _, err := s.activities.GoPayAppSaveStateActivity(ctx, pb.GoPayAppStateActivityInput{JobId: jobID, UserId: userID, StateJson: stateJSON, Reason: "payment_rebind_login_ready"}); err != nil {
		return s.markActionFailed(ctx, jobID, "save_gopay_state", jobstatus.FailedRetryable, false, true, err, combined)
	}

	changePhone, err := s.runGoPayAppChangePhone(ctx, jobID, stateJSON, pin, params["country_code"])
	mergeActionData(combined, "change_phone", structMap(changePhone.GetData()))
	if nextStateJSON := strings.TrimSpace(changePhone.GetStateJson()); nextStateJSON != "" {
		stateJSON = nextStateJSON
		_, _ = s.activities.GoPayAppSaveStateActivity(ctx, pb.GoPayAppStateActivityInput{JobId: jobID, UserId: userID, StateJson: stateJSON, Reason: "payment_rebind_attempt"})
	}
	if err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayAppChangePhone, jobstatus.FailedRetryable, false, true, err, combined)
	}
	if !changePhone.GetChangePhoneComplete() {
		err := fmt.Errorf("gopay payment rebind did not complete")
		return s.markActionFailed(ctx, jobID, stepGoPayAppChangePhone, jobstatus.FailedRetryable, false, true, err, combined)
	}
	if err := s.finishGoPayChangePhoneSMS(ctx, jobID, changePhone.GetActivationId(), "payment_rebind_retry_complete"); err != nil {
		return s.markActionFailed(ctx, jobID, stepGoPayAppSMSFinish, jobstatus.FailedRetryable, false, true, err, combined)
	}
	_, _ = s.activities.GoPayAppDeleteStateActivity(ctx, pb.GoPayAppStateActivityInput{JobId: jobID, UserId: userID, Reason: "payment_rebind_complete"})

	combined["activation_id"] = changePhone.GetActivationId()
	combined["bound_phone_present"] = strings.TrimSpace(changePhone.GetPhone()) != ""
	combined["change_phone_complete"] = changePhone.GetChangePhoneComplete()
	return s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(combined)})
}

func (s *Server) runGoPayAppAuth(ctx context.Context, jobID string, opts goPayAppActionOptions) (pb.GoPayAppStepOutput, error) {
	token, err := s.runGoPayAppEnsureTokenAvailable(ctx, jobID, opts)
	if err != nil {
		return token, err
	}
	pinOpts := opts
	pinOpts.StateJSON = token.GetStateJson()
	pin, err := s.runGoPayAppEnsurePINSetup(ctx, jobID, pinOpts)
	if err != nil {
		return pin, err
	}
	if pin.GetReady() || pin.GetAccountTokenReady() {
		return pin, nil
	}
	return pin, fmt.Errorf("gopay auth did not reach token-valid state after ensure pin setup")
}

func (s *Server) runGoPayAppEnsureTokenAvailable(ctx context.Context, jobID string, opts goPayAppActionOptions) (pb.GoPayAppStepOutput, error) {
	var last pb.GoPayAppOTPOutput
	stateJSON := opts.StateJSON
	for attempt := 0; attempt < 4; attempt++ {
		current, err := s.activities.GoPayAppOTPStartActivity(ctx, pb.GoPayAppOTPStartInput{JobId: jobID, Operation: "auth", StepName: stepGoPayAppLogin, Phone: opts.Phone, OtpChannel: opts.OTPChannel, SmsActivationId: opts.SMSActivationID, StateJson: stateJSON, Pin: opts.Pin, CountryCode: opts.CountryCode})
		last = current
		if err != nil {
			return goPayAppStepFromOTP(last), err
		}
		stateJSON = last.GetStateJson()
		if last.GetReady() || last.GetAccountTokenReady() {
			return goPayAppStepFromOTP(last), nil
		}
		if !last.GetOtpRequired() {
			continue
		}

		startChannel := effectiveGoPayOTPChannel(last, opts.OTPChannel)
		otp, err := s.waitGoPayOTP(ctx, goPayOTPWaitInput(jobID, stepGoPayAppLogin, last, startChannel, opts.SMSActivationID, opts.Source))
		if err != nil {
			return goPayAppStepFromOTP(last), err
		}
		completed, err := s.activities.GoPayAppOTPCompleteActivity(ctx, pb.GoPayAppOTPCompleteInput{JobId: jobID, Operation: "auth", OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam, IssuedAfterUnix: last.GetIssuedAfterUnix(), OtpSource: otp.GetSource(), Data: last.GetData(), OtpChannel: startChannel, SmsActivationId: opts.SMSActivationID, StateJson: stateJSON, Pin: opts.Pin})
		last = completed
		if err != nil {
			return goPayAppStepFromOTP(last), err
		}
		stateJSON = last.GetStateJson()
		if last.GetReady() || last.GetAccountTokenReady() {
			return goPayAppStepFromOTP(last), nil
		}
	}
	return goPayAppStepFromOTP(last), fmt.Errorf("gopay auth did not reach token-valid state")
}

func (s *Server) runGoPayAppEnsurePINSetup(ctx context.Context, jobID string, opts goPayAppActionOptions) (pb.GoPayAppStepOutput, error) {
	start, err := s.activities.GoPayAppCreatePinStartActivity(ctx, pb.GoPayAppCreatePinStartInput{JobId: jobID, OtpChannel: opts.OTPChannel, SmsActivationId: opts.SMSActivationID, StateJson: opts.StateJSON, Pin: opts.Pin})
	if err != nil {
		return goPayAppStepFromOTP(start), err
	}
	if start.GetReady() || start.GetAccountTokenReady() || start.GetSignupPinComplete() {
		return goPayAppStepFromOTP(start), nil
	}
	if !start.GetOtpRequired() {
		return goPayAppStepFromOTP(start), fmt.Errorf("gopay create pin did not request OTP and did not become ready")
	}
	startChannel := effectiveGoPayOTPChannel(start, opts.OTPChannel)
	var otp pb.OTPWaitOutput
	for attempt := 0; attempt < 2; attempt++ {
		if startChannel == "sms" {
			if _, err := s.activities.GoPayAppSMSRequestAdditionalCodeActivity(ctx, pb.GoPayAppSMSActivationInput{JobId: jobID, ActivationId: opts.SMSActivationID, Reason: stepGoPayAppEnsurePINSetup}); err != nil {
				return goPayAppStepFromOTP(start), err
			}
		}
		current, err := s.waitGoPayOTP(ctx, goPayOTPWaitInput(jobID, stepGoPayAppEnsurePINSetup, start, startChannel, opts.SMSActivationID, opts.Source))
		otp = current
		if err != nil {
			if !isOTPWaitNotReceivedError(err) {
				return goPayAppStepFromOTP(start), err
			}
			otp = pb.OTPWaitOutput{ErrorMessage: err.Error(), Data: current.GetData()}
		}
		if otp.GetFound() {
			break
		}
		if attempt == 1 {
			return goPayAppStepFromOTP(start), goPayEnsurePINSetupOTPNotReceivedError(otp)
		}
		retry, err := s.activities.GoPayAppCreatePinRetryActivity(ctx, pb.GoPayAppCreatePinStartInput{JobId: jobID, OtpChannel: startChannel, Data: start.GetData(), SmsActivationId: opts.SMSActivationID, StateJson: start.GetStateJson(), Pin: opts.Pin})
		start = retry
		if err != nil {
			return goPayAppStepFromOTP(start), err
		}
		if !start.GetOtpRequired() {
			return goPayAppStepFromOTP(start), fmt.Errorf("gopay create pin retry did not request OTP")
		}
	}
	completed, err := s.activities.GoPayAppCreatePinCompleteActivity(ctx, pb.GoPayAppCreatePinCompleteInput{JobId: jobID, OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam, IssuedAfterUnix: start.GetIssuedAfterUnix(), OtpSource: otp.GetSource(), Data: start.GetData(), OtpChannel: startChannel, SmsActivationId: opts.SMSActivationID, StateJson: start.GetStateJson(), Pin: opts.Pin})
	return goPayAppStepFromOTP(completed), err
}

func (s *Server) runGoPayAppChangePhone(ctx context.Context, jobID string, stateJSON string, pin string, countryCode string) (pb.GoPayAppStepOutput, error) {
	var failureCount int32
	var last pb.GoPayAppStepOutput
	for {
		number, err := s.activities.GoPayAppChangePhoneGetNumberActivity(ctx, pb.GoPayAppChangePhoneGetNumberInput{JobId: jobID, FailureCount: failureCount})
		failureCount = number.GetFailureCount()
		last = pb.GoPayAppStepOutput{ActivationId: number.GetActivationId(), Phone: number.GetPhone(), Data: number.GetData(), StateJson: stateJSON}
		if err != nil {
			return last, err
		}

		start, err := s.activities.GoPayAppChangePhoneStartActivity(ctx, pb.GoPayAppChangePhoneStartInput{JobId: jobID, FailureCount: failureCount, StateJson: stateJSON, ActivationId: number.GetActivationId(), Phone: number.GetPhone(), Pin: pin, CountryCode: countryCode})
		stateJSON = start.GetStateJson()
		failureCount = start.GetFailureCount()
		last = pb.GoPayAppStepOutput{ActivationId: start.GetActivationId(), Phone: start.GetPhone(), Data: start.GetData(), StateJson: stateJSON}
		if err != nil {
			return last, err
		}
		if start.GetErrorMessage() != "" {
			canceled, cancelErr := s.activities.GoPayAppSMSCancelBeforeRotationActivity(ctx, pb.GoPayAppSMSActivationInput{JobId: jobID, ActivationId: start.GetActivationId(), FailureCount: failureCount, Reason: start.GetErrorMessage()})
			failureCount = canceled.GetFailureCount()
			last.ActivationId = canceled.GetActivationId()
			last.Data = canceled.GetData()
			last.StateJson = stateJSON
			if cancelErr != nil {
				return last, cancelErr
			}
			if start.GetRetryableFailure() {
				continue
			}
			return last, fmt.Errorf("%s", start.GetErrorMessage())
		}
		if start.GetRetryableFailure() {
			continue
		}

		for otpAttempt := int32(0); otpAttempt <= start.GetOtpRetryAttempts(); otpAttempt++ {
			issuedAfterUnix := time.Now().Unix()
			wait, err := s.waitGoPayOTP(ctx, pb.OTPWaitInput{JobId: jobID, StepName: stepGoPayAppChangePhoneSMSWait, Target: &pb.OTPWaitInput_Sms{Sms: &pb.OTPWaitSMSTarget{ActivationId: start.GetActivationId()}}, TimeoutSeconds: start.GetOtpTimeoutSeconds(), IssuedAfterUnix: issuedAfterUnix, OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam})
			if err != nil {
				_, _ = s.activities.GoPayAppSMSCancelBeforeRotationActivity(ctx, pb.GoPayAppSMSActivationInput{JobId: jobID, ActivationId: start.GetActivationId(), FailureCount: failureCount, Reason: err.Error()})
				return last, err
			}
			if wait.GetFound() {
				complete, err := s.activities.GoPayAppChangePhoneCompleteActivity(ctx, pb.GoPayAppChangePhoneCompleteInput{JobId: jobID, ActivationId: start.GetActivationId(), Code: wait.GetCode(), FailureCount: failureCount, StateJson: stateJSON, OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam, IssuedAfterUnix: issuedAfterUnix})
				last = goPayAppStepFromChangePhoneComplete(complete)
				stateJSON = last.GetStateJson()
				if err != nil {
					return last, err
				}
				failureCount = complete.GetFailureCount()
				if complete.GetChangePhoneComplete() {
					return last, nil
				}
				if complete.GetRetryableFailure() {
					break
				}
				return last, fmt.Errorf("gopay change phone did not complete")
			}

			if otpAttempt < start.GetOtpRetryAttempts() {
				retry, err := s.activities.GoPayAppChangePhoneRetryActivity(ctx, pb.GoPayAppChangePhoneRetryInput{JobId: jobID, ActivationId: start.GetActivationId(), OtpAttempt: otpAttempt + 1, StateJson: stateJSON})
				if err != nil {
					_, _ = s.activities.GoPayAppSMSCancelBeforeRotationActivity(ctx, pb.GoPayAppSMSActivationInput{JobId: jobID, ActivationId: start.GetActivationId(), FailureCount: failureCount, Reason: err.Error()})
					return last, err
				}
				stateJSON = retry.GetStateJson()
				if retry.GetOtpSent() {
					continue
				}
				if retry.GetErrorMessage() != "" {
					wait.ErrorMessage = "ChangePhoneRetry: " + retry.GetErrorMessage()
				}
			}

			reason := wait.GetErrorMessage()
			if reason == "" {
				reason = "WaitCode: otp not found"
			} else {
				reason = "WaitCode: " + reason
			}
			canceled, cancelErr := s.activities.GoPayAppSMSCancelBeforeRotationActivity(ctx, pb.GoPayAppSMSActivationInput{JobId: jobID, ActivationId: start.GetActivationId(), FailureCount: failureCount, Reason: reason})
			failureCount = canceled.GetFailureCount()
			last.ActivationId = canceled.GetActivationId()
			last.Data = canceled.GetData()
			last.StateJson = stateJSON
			if cancelErr != nil {
				return last, cancelErr
			}
			break
		}
	}
}

func (s *Server) finishGoPayChangePhoneSMS(ctx context.Context, jobID, activationID, reason string) error {
	if strings.TrimSpace(activationID) == "" {
		return fmt.Errorf("change phone activation id is missing")
	}
	_, err := s.activities.GoPayAppSMSFinishActivity(ctx, pb.GoPayAppSMSActivationInput{JobId: jobID, ActivationId: activationID, Reason: reason})
	return err
}

func (s *Server) waitGoPayOTP(ctx context.Context, input pb.OTPWaitInput) (pb.OTPWaitOutput, error) {
	channel := otpwait.Channel(&input)
	if channel == "" {
		return pb.OTPWaitOutput{}, fmt.Errorf("otp wait target missing")
	}
	timeoutSeconds := input.GetTimeoutSeconds()
	if timeoutSeconds <= 0 {
		timeoutSeconds = 180
	}
	input.TimeoutSeconds = timeoutSeconds
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	lastErr := ""
	for {
		manual, err := s.activities.FetchManualOTPActivity(ctx, input)
		if err != nil {
			lastErr = err.Error()
		} else if manual.GetFound() {
			return manual, nil
		}
		remaining := int32(time.Until(deadline).Seconds())
		if remaining <= 0 {
			break
		}
		chunk := input
		chunk.TimeoutSeconds = minOTPWaitChunkSeconds(remaining)
		out, err := s.activities.OTPWaitActivity(ctx, chunk)
		if err != nil {
			lastErr = err.Error()
		} else if out.GetFound() {
			return out, nil
		}
		select {
		case <-ctx.Done():
			return pb.OTPWaitOutput{}, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	if lastErr != "" {
		return pb.OTPWaitOutput{ErrorMessage: lastErr}, fmt.Errorf("otp not received after %ds: %s", timeoutSeconds, lastErr)
	}
	return pb.OTPWaitOutput{}, fmt.Errorf("otp not received after %ds", timeoutSeconds)
}

func goPayAppStepFromOTP(output pb.GoPayAppOTPOutput) pb.GoPayAppStepOutput {
	return pb.GoPayAppStepOutput{Ready: output.GetReady(), Stage: output.GetStage(), Phone: output.GetPhone(), AccountTokenReady: output.GetAccountTokenReady(), SignupComplete: output.GetSignupComplete(), SignupPinComplete: output.GetSignupPinComplete(), Data: output.GetData(), StateJson: output.GetStateJson()}
}

func goPayAppStepFromChangePhoneComplete(output pb.GoPayAppChangePhoneCompleteOutput) pb.GoPayAppStepOutput {
	return pb.GoPayAppStepOutput{ActivationId: output.GetActivationId(), Stage: output.GetStage(), Phone: output.GetPhone(), ChangePhoneComplete: output.GetChangePhoneComplete(), Data: output.GetData(), StateJson: output.GetStateJson()}
}

func goPayOTPWaitInput(jobID, stepName string, start pb.GoPayAppOTPOutput, channel string, activationID string, source string) pb.OTPWaitInput {
	input := pb.OTPWaitInput{JobId: jobID, StepName: stepName, TimeoutSeconds: start.GetTimeoutSeconds(), IssuedAfterUnix: start.GetIssuedAfterUnix(), OtpParam: paymentOTPParam, SubmittedAtParam: paymentOTPSubmittedAtParam}
	if effectiveGoPayOTPChannel(start, channel) == "sms" {
		input.Target = &pb.OTPWaitInput_Sms{Sms: &pb.OTPWaitSMSTarget{ActivationId: activationID}}
		return input
	}
	if strings.TrimSpace(source) == "" {
		source = goPayLocalSource
	}
	input.Target = &pb.OTPWaitInput_Payment{Payment: &pb.OTPWaitPaymentTarget{Source: source}}
	return input
}

func effectiveGoPayOTPChannel(start pb.GoPayAppOTPOutput, requested string) string {
	if channel := normalizeGoPayOTPChannel(requested); channel != "" {
		return channel
	}
	return normalizeGoPayOTPChannel(start.GetOtpChannel())
}

func normalizeGoPayOTPChannel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "wa", "whatsapp", "otp_wa":
		return "wa"
	case "sms", "otp_sms":
		return "sms"
	default:
		return ""
	}
}

func isOTPWaitNotReceivedError(err error) bool {
	if err == nil {
		return false
	}
	normalized := strings.ToLower(err.Error())
	return strings.Contains(normalized, "otp not received") || strings.Contains(normalized, "otp not found") || strings.Contains(normalized, "waitcode")
}

func goPayEnsurePINSetupOTPNotReceivedError(wait pb.OTPWaitOutput) error {
	reason := wait.GetErrorMessage()
	if reason == "" {
		reason = "otp not found"
	}
	return fmt.Errorf("gopay ensure pin setup otp not received: %s", reason)
}
