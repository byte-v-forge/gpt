package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

const (
	codexOAuthProtocolMode         = "codex_oauth"
	codexOAuthProtocolAddPhoneMode = "codex_oauth_add_phone"
	codexOAuthMaxPhoneAttempts     = 3
	codexOAuthMaxAcquireAttempts   = 3
)

type codexOAuthActionInput struct {
	JobID                       string
	AccountID                   string
	Label                       string
	Phone                       *pb.CodexOAuthPhoneLease
	AllowAddPhone               bool
	MarkPhoneConfirmedOnSuccess bool
	MaxReuseCount               int32
}

type codexOAuthRun struct {
	authSecretKey     string
	phoneLabel        string
	phoneReuseCount   int32
	phoneReuseLimit   int32
	addPhoneConfirmed bool
	addPhoneRequired  bool
	data              map[string]any
}

type codexOAuthAddPhoneAttempt struct {
	run   codexOAuthRun
	phone *pb.CodexOAuthPhoneLease
}

type codexOAuthBatchResult struct {
	TotalCount          int32
	SuccessCount        int32
	AddPhoneCount       int32
	DirectOauthCount    int32
	StoppedReason       string
	PhoneLabel          string
	PhoneReuseCount     int32
	PhoneReuseLimit     int32
	ProcessedAccountIDs []string
}

func (s *Server) runCodexOAuthAction(ctx context.Context, jobID string, accountID string, params map[string]string) error {
	account, err := s.resolveCodexOAuthActionAccount(ctx, jobID, accountID)
	if err != nil {
		return err
	}
	run, failedStep, err := s.runCodexOAuthBrowser(ctx, codexOAuthActionInput{JobID: jobID, AccountID: account.GetAccountId(), Label: params["label"]})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "add_phone_required") {
			run.addPhoneRequired = true
		}
		return s.markActionFailed(ctx, jobID, failedStep, jobstatus.FailedRecoverable, true, false, err, run.resultData(account.GetAccountId()))
	}
	return s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(run.resultData(account.GetAccountId()))})
}

func (s *Server) runCodexOAuthProtocolAction(ctx context.Context, jobID string, accountID string, params map[string]string) error {
	account, err := s.resolveCodexOAuthActionAccount(ctx, jobID, accountID)
	if err != nil {
		return err
	}
	run, failedStep, err := s.runCodexOAuthProtocol(ctx, codexOAuthActionInput{JobID: jobID, AccountID: account.GetAccountId(), Label: params["label"]})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "add_phone_required") {
			run.addPhoneRequired = true
		}
		return s.markActionFailed(ctx, jobID, failedStep, jobstatus.FailedRecoverable, true, false, err, run.resultData(account.GetAccountId()))
	}
	return s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(run.resultData(account.GetAccountId()))})
}

func (s *Server) runCodexOAuthAddPhoneAction(ctx context.Context, jobID string, accountID string, params map[string]string) error {
	account, err := s.resolveCodexOAuthActionAccount(ctx, jobID, accountID)
	if err != nil {
		return err
	}
	attempt, failedStep, err := s.runCodexOAuthAddPhoneWithRotation(ctx, codexOAuthActionInput{JobID: jobID, AccountID: account.GetAccountId(), Label: params["label"], MaxReuseCount: int32Param(params, "max_reuse_count", 0)})
	if err != nil {
		return s.markActionFailed(ctx, jobID, failedStep, jobstatus.FailedRetryable, false, true, err, attempt.run.resultData(account.GetAccountId()))
	}
	return s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(attempt.run.resultData(account.GetAccountId()))})
}

func (s *Server) runCodexOAuthBatchAddPhoneAction(ctx context.Context, jobID string, params map[string]string) error {
	accountIDs := compactAccountIDs(strings.Split(params["account_ids"], ","))
	if len(accountIDs) == 0 {
		err := fmt.Errorf("account_ids is required")
		return s.markActionFailed(ctx, jobID, "codex_oauth_batch_input", jobstatus.FailedFinal, false, false, err, nil)
	}
	label := params["label"]
	maxReuseCount := int32Param(params, "max_reuse_count", 0)
	result := codexOAuthBatchResult{TotalCount: int32(len(accountIDs))}
	for _, accountID := range accountIDs {
		attempt, failedStep, err := s.runCodexOAuthAddPhoneWithRotation(ctx, codexOAuthActionInput{JobID: jobID, AccountID: accountID, Label: label, MaxReuseCount: maxReuseCount})
		if err != nil {
			if reason := codexOAuthBatchStopReason(err.Error()); reason != "" {
				result.StoppedReason = reason
				return s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(result.data())})
			}
			return s.markActionFailed(ctx, jobID, failedStep, jobstatus.FailedRetryable, false, true, err, mergeCodexOAuthBatchFailureData(result.data(), attempt.run.data))
		}
		result.SuccessCount++
		result.ProcessedAccountIDs = append(result.ProcessedAccountIDs, accountID)
		if attempt.run.addPhoneConfirmed {
			result.AddPhoneCount++
		} else {
			result.DirectOauthCount++
		}
		result.PhoneLabel = attempt.run.phoneLabel
		result.PhoneReuseCount = attempt.run.phoneReuseCount
		result.PhoneReuseLimit = attempt.run.phoneReuseLimit
	}
	return s.activities.MarkJobSucceededActivity(ctx, pb.JobSuccessInput{JobId: jobID, Result: structData(result.data())})
}

func (s *Server) resolveCodexOAuthActionAccount(ctx context.Context, jobID string, accountID string) (pb.AccountRef, error) {
	account, err := s.activities.ResolveAccountFromJobActivity(ctx, pb.ResolveAccountInput{AccountId: strings.TrimSpace(accountID)})
	if err != nil {
		return account, s.markActionFailed(ctx, jobID, "resolve_account", jobstatus.FailedRetryable, false, true, err, nil)
	}
	return account, nil
}

func (s *Server) runCodexOAuthBrowser(ctx context.Context, input codexOAuthActionInput) (codexOAuthRun, string, error) {
	run := codexOAuthRun{phoneLabel: input.Label, data: map[string]any{"driver": "browser"}}
	if input.Phone != nil {
		run.phoneReuseCount = input.Phone.GetReuseCount()
		run.phoneReuseLimit = input.Phone.GetReuseLimit()
	}
	var session *pb.CodexOAuthBrowserSession
	defer func() {
		if session != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = s.activities.CodexOAuthStopBrowserActivity(cleanupCtx, pb.CodexOAuthStopBrowserInput{JobId: input.JobID, Session: session, Reason: "codex oauth browser cleanup"})
		}
	}()

	start, err := s.activities.CodexOAuthStartBrowserActivity(ctx, pb.CodexOAuthStartBrowserInput{JobId: input.JobID, AccountId: input.AccountID, Label: input.Label, Phone: input.Phone, AllowAddPhone: input.AllowAddPhone})
	mergeActionData(run.data, stepCodexOAuthBrowserStart, structMap(start.GetData()))
	if start.GetPhoneLabel() != "" {
		run.phoneLabel = start.GetPhoneLabel()
	}
	if err != nil {
		return run, stepCodexOAuthBrowserStart, err
	}
	session = start.GetSession()
	if session == nil {
		return run, stepCodexOAuthBrowserStart, fmt.Errorf("codex oauth browser session missing")
	}

	stage, _, failedStep, err := s.runCodexOAuthBrowserLoginStages(ctx, input, session, run.data)
	if err != nil {
		return run, failedStep, err
	}
	if input.Phone != nil || stage == "add_phone" {
		addPhone, err := s.activities.CodexOAuthAddPhoneBrowserActivity(ctx, pb.CodexOAuthAddPhoneBrowserInput{JobId: input.JobID, AccountId: input.AccountID, Label: input.Label, Phone: input.Phone, AllowAddPhone: input.AllowAddPhone, Session: session})
		mergeActionData(run.data, stepCodexOAuthBrowserAddPhone, structMap(addPhone.GetData()))
		run.addPhoneConfirmed = addPhone.GetAddPhoneConfirmed()
		run.addPhoneRequired = addPhone.GetAddPhoneRequired() || (err != nil && strings.Contains(strings.ToLower(err.Error()), "add_phone_required"))
		run.phoneReuseCount = addPhone.GetPhoneReuseCount()
		run.phoneReuseLimit = addPhone.GetPhoneReuseLimit()
		if err != nil {
			return run, stepCodexOAuthBrowserAddPhone, err
		}
		stage, _, failedStep, err = s.runCodexOAuthBrowserLoginStages(ctx, input, session, run.data)
		if err != nil {
			return run, failedStep, err
		}
		if stage == "add_phone" {
			return run, stepCodexOAuthBrowserAddPhone, fmt.Errorf("phone_rejected: add phone still required after otp submit")
		}
	}

	complete, err := s.activities.CodexOAuthCompleteBrowserActivity(ctx, pb.CodexOAuthCompleteBrowserInput{JobId: input.JobID, AccountId: input.AccountID, Label: input.Label, MarkPhoneConfirmedOnSuccess: input.MarkPhoneConfirmedOnSuccess, Session: session})
	mergeActionData(run.data, stepCodexOAuthBrowserComplete, structMap(complete.GetData()))
	if err != nil {
		return run, stepCodexOAuthBrowserComplete, err
	}
	run.authSecretKey = complete.GetAuthSecretKey()
	return run, "", nil
}

func (s *Server) runCodexOAuthProtocol(ctx context.Context, input codexOAuthActionInput) (codexOAuthRun, string, error) {
	run := codexOAuthRun{phoneLabel: input.Label, data: map[string]any{"driver": "protocol"}}
	var session *pb.CodexOAuthBrowserSession
	defer func() {
		if session != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = s.activities.CodexOAuthStopProtocolActivity(cleanupCtx, pb.CodexOAuthStopBrowserInput{JobId: input.JobID, Session: session, Reason: "codex oauth protocol cleanup"})
		}
	}()

	proxy, err := s.activities.ProtocolUseProxyActivity(ctx, pb.ProtocolAuthStartInput{JobId: input.JobID, AccountId: input.AccountID, Mode: codexOAuthProtocolMode})
	mergeActionData(run.data, stepProtocolUseProxy, structMap(proxy.GetData()))
	if err != nil {
		return run, stepProtocolUseProxy, err
	}

	start, err := s.activities.CodexOAuthStartProtocolActivity(ctx, pb.CodexOAuthStartBrowserInput{JobId: input.JobID, AccountId: input.AccountID, Label: input.Label})
	mergeActionData(run.data, stepCodexOAuthProtocolStart, structMap(start.GetData()))
	if start.GetPhoneLabel() != "" {
		run.phoneLabel = start.GetPhoneLabel()
	}
	if err != nil {
		return run, stepCodexOAuthProtocolStart, err
	}
	session = start.GetSession()
	if session == nil {
		return run, stepCodexOAuthProtocolStart, fmt.Errorf("codex oauth protocol session missing")
	}

	stage, _, failedStep, err := s.runCodexOAuthProtocolLoginStages(ctx, input, session, run.data)
	if err != nil {
		if stage == "add_phone" || strings.Contains(strings.ToLower(err.Error()), "add_phone_required") {
			run.addPhoneRequired = true
		}
		return run, failedStep, err
	}
	if stage == "add_phone" {
		run.addPhoneRequired = true
		return run, stepCodexOAuthProtocolDetect, fmt.Errorf("codex_oauth_add_phone_required")
	}

	complete, err := s.activities.CodexOAuthCompleteProtocolActivity(ctx, pb.CodexOAuthCompleteBrowserInput{JobId: input.JobID, AccountId: input.AccountID, Label: input.Label, Session: session})
	mergeActionData(run.data, stepCodexOAuthProtocolComplete, structMap(complete.GetData()))
	if err != nil {
		return run, stepCodexOAuthProtocolComplete, err
	}
	run.authSecretKey = complete.GetAuthSecretKey()
	return run, "", nil
}

func (s *Server) runCodexOAuthAddPhoneWithRotation(ctx context.Context, input codexOAuthActionInput) (codexOAuthAddPhoneAttempt, string, error) {
	var last codexOAuthAddPhoneAttempt
	for attempt := 1; attempt <= codexOAuthMaxPhoneAttempts; attempt++ {
		var failedStep string
		var err error
		last, failedStep, err = s.runCodexOAuthAddPhoneAttempt(ctx, input, attempt)
		last.run.data["phone_attempt"] = attempt
		last.run.data["phone_max_attempts"] = codexOAuthMaxPhoneAttempts
		if err == nil {
			return last, "", nil
		}
		if reason := codexOAuthPhoneRetryReason(err.Error()); reason != "" {
			last.run.data["phone_retry_reason"] = reason
		}
		s.releaseCodexOAuthAttemptPhone(input, last.phone, err)
		if reason := codexOAuthPhoneRetryReason(err.Error()); reason == "" || attempt >= codexOAuthMaxPhoneAttempts {
			return last, failedStep, err
		}
	}
	return last, stepCodexOAuthAcquirePhone, fmt.Errorf("codex oauth add phone failed after %d phone attempts", codexOAuthMaxPhoneAttempts)
}

func (s *Server) runCodexOAuthAddPhoneAttempt(ctx context.Context, input codexOAuthActionInput, attempt int) (codexOAuthAddPhoneAttempt, string, error) {
	run := codexOAuthRun{phoneLabel: input.Label, data: map[string]any{"phone_attempt": attempt, "phone_max_attempts": codexOAuthMaxPhoneAttempts, "driver": "protocol"}}
	result := codexOAuthAddPhoneAttempt{run: run}
	var session *pb.CodexOAuthBrowserSession
	defer func() {
		if session != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = s.activities.CodexOAuthStopProtocolActivity(cleanupCtx, pb.CodexOAuthStopBrowserInput{JobId: input.JobID, Session: session, Reason: "codex oauth protocol cleanup"})
		}
	}()

	proxy, err := s.activities.ProtocolUseProxyActivity(ctx, pb.ProtocolAuthStartInput{JobId: input.JobID, AccountId: input.AccountID, Mode: codexOAuthProtocolAddPhoneMode})
	mergeActionData(result.run.data, stepProtocolUseProxy, structMap(proxy.GetData()))
	if err != nil {
		return result, stepProtocolUseProxy, err
	}

	start, err := s.activities.CodexOAuthStartProtocolActivity(ctx, pb.CodexOAuthStartBrowserInput{JobId: input.JobID, AccountId: input.AccountID, Label: input.Label, AllowAddPhone: true})
	mergeActionData(result.run.data, stepCodexOAuthProtocolStart, structMap(start.GetData()))
	if start.GetPhoneLabel() != "" {
		result.run.phoneLabel = start.GetPhoneLabel()
	}
	if err != nil {
		return result, stepCodexOAuthProtocolStart, err
	}
	session = start.GetSession()
	if session == nil {
		return result, stepCodexOAuthProtocolStart, fmt.Errorf("codex oauth protocol session missing")
	}

	stage, _, failedStep, err := s.runCodexOAuthProtocolLoginStages(ctx, codexOAuthActionInput{JobID: input.JobID, AccountID: input.AccountID, Label: input.Label, AllowAddPhone: true, MarkPhoneConfirmedOnSuccess: true}, session, result.run.data)
	if err != nil {
		return result, failedStep, err
	}
	if stage == "add_phone" {
		phone, failedStep, err := s.acquireCodexOAuthPhoneAfterLogin(ctx, input)
		result.phone = phone
		mergeActionData(result.run.data, "codex_oauth_phone", codexOAuthPhoneLeaseRunData(phone))
		if phone != nil {
			result.run.phoneReuseCount = phone.GetReuseCount()
			result.run.phoneReuseLimit = phone.GetReuseLimit()
		}
		if err != nil {
			if reason := codexOAuthPhoneSupplyStopReason(err.Error()); reason != "" {
				result.run.data["phone_stop_reason"] = reason
			}
			return result, failedStep, err
		}
		addPhone, err := s.activities.CodexOAuthAddPhoneProtocolActivity(ctx, pb.CodexOAuthAddPhoneBrowserInput{JobId: input.JobID, AccountId: input.AccountID, Label: input.Label, Phone: phone, AllowAddPhone: true, Session: session})
		mergeActionData(result.run.data, stepCodexOAuthProtocolAddPhone, structMap(addPhone.GetData()))
		result.run.addPhoneConfirmed = addPhone.GetAddPhoneConfirmed()
		result.run.addPhoneRequired = addPhone.GetAddPhoneRequired() || (err != nil && strings.Contains(strings.ToLower(err.Error()), "add_phone_required"))
		result.run.phoneReuseCount = addPhone.GetPhoneReuseCount()
		result.run.phoneReuseLimit = addPhone.GetPhoneReuseLimit()
		if err != nil {
			return result, stepCodexOAuthProtocolAddPhone, err
		}
		stage, _, failedStep, err = s.runCodexOAuthProtocolLoginStages(ctx, codexOAuthActionInput{JobID: input.JobID, AccountID: input.AccountID, Label: input.Label, AllowAddPhone: true, MarkPhoneConfirmedOnSuccess: true}, session, result.run.data)
		if err != nil {
			return result, failedStep, err
		}
		if stage == "add_phone" {
			return result, stepCodexOAuthProtocolAddPhone, fmt.Errorf("phone_rejected: add phone still required after otp submit")
		}
	}

	complete, err := s.activities.CodexOAuthCompleteProtocolActivity(ctx, pb.CodexOAuthCompleteBrowserInput{JobId: input.JobID, AccountId: input.AccountID, Label: input.Label, MarkPhoneConfirmedOnSuccess: true, Session: session})
	mergeActionData(result.run.data, stepCodexOAuthProtocolComplete, structMap(complete.GetData()))
	if err != nil {
		return result, stepCodexOAuthProtocolComplete, err
	}
	result.run.authSecretKey = complete.GetAuthSecretKey()
	return result, "", nil
}

func (s *Server) acquireCodexOAuthPhoneAfterLogin(ctx context.Context, input codexOAuthActionInput) (*pb.CodexOAuthPhoneLease, string, error) {
	var phone *pb.CodexOAuthPhoneLease
	var lastErr error
	for attempt := 1; attempt <= codexOAuthMaxAcquireAttempts; attempt++ {
		lease, err := s.activities.CodexOAuthAcquirePhoneActivity(ctx, pb.CodexOAuthAcquirePhoneInput{JobId: input.JobID, AccountId: input.AccountID, Label: input.Label, MaxReuseCount: input.MaxReuseCount})
		phone = lease
		if err == nil {
			return phone, "", nil
		}
		lastErr = err
		if reason := codexOAuthPhoneSupplyStopReason(err.Error()); reason == "" || attempt >= codexOAuthMaxAcquireAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return phone, stepCodexOAuthAcquirePhone, ctx.Err()
		case <-time.After(time.Duration(5*attempt) * time.Second):
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("acquire phone failed")
	}
	if reason := codexOAuthPhoneSupplyStopReason(lastErr.Error()); reason != "" {
		return phone, stepCodexOAuthAcquirePhone, fmt.Errorf("%s: %s", reason, codexOAuthCleanAcquirePhoneError(lastErr.Error()))
	}
	return phone, stepCodexOAuthAcquirePhone, lastErr
}

func (s *Server) releaseCodexOAuthAttemptPhone(input codexOAuthActionInput, phone *pb.CodexOAuthPhoneLease, err error) {
	if err == nil || phone == nil || strings.TrimSpace(phone.GetActivationId()) == "" {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	_ = s.activities.CodexOAuthReleasePhoneActivity(cleanupCtx, pb.CodexOAuthReleasePhoneInput{JobId: input.JobID, AccountId: input.AccountID, ActivationId: phone.GetActivationId(), Label: input.Label, ErrorMessage: err.Error()})
}

func (s *Server) runCodexOAuthBrowserLoginStages(ctx context.Context, input codexOAuthActionInput, session *pb.CodexOAuthBrowserSession, data map[string]any) (string, int64, string, error) {
	stepInput := pb.CodexOAuthBrowserStepInput{JobId: input.JobID, AccountId: input.AccountID, Label: input.Label, Session: session}
	stage, issuedAfter, failedStep, err := s.runCodexOAuthStageActivity(stepCodexOAuthBrowserDetect, data, func() (pb.CodexOAuthBrowserStageOutput, error) {
		return s.activities.CodexOAuthDetectBrowserStageActivity(ctx, stepInput)
	})
	if err != nil {
		return stage, issuedAfter, failedStep, err
	}
	if stage == "email" {
		stage, issuedAfter, failedStep, err = s.runCodexOAuthStageActivity(stepCodexOAuthBrowserEmail, data, func() (pb.CodexOAuthBrowserStageOutput, error) {
			return s.activities.CodexOAuthSubmitEmailActivity(ctx, stepInput)
		})
		if err != nil {
			return stage, issuedAfter, failedStep, err
		}
	}
	if stage == "password" {
		stage, issuedAfter, failedStep, err = s.runCodexOAuthStageActivity(stepCodexOAuthBrowserPassword, data, func() (pb.CodexOAuthBrowserStageOutput, error) {
			return s.activities.CodexOAuthSubmitPasswordActivity(ctx, stepInput)
		})
		if err != nil {
			return stage, issuedAfter, failedStep, err
		}
	}
	if stage == "email_otp" {
		issuedAfter = codexOAuthEmailOTPIssuedAfter(issuedAfter)
		stage, issuedAfter, failedStep, err = s.runCodexOAuthStageActivity(stepCodexOAuthBrowserEmailOTP, data, func() (pb.CodexOAuthBrowserStageOutput, error) {
			return s.activities.CodexOAuthSubmitEmailOTPActivity(ctx, pb.CodexOAuthSubmitEmailOTPInput{JobId: input.JobID, AccountId: input.AccountID, Label: input.Label, Session: session, IssuedAfterUnix: issuedAfter})
		})
		if err != nil {
			return stage, issuedAfter, failedStep, err
		}
	}
	if !codexOAuthBrowserReadyStage(stage) {
		return stage, issuedAfter, codexOAuthBrowserStageStepName(stage), fmt.Errorf("codex oauth login stage not ready: %s", stage)
	}
	return stage, issuedAfter, "", nil
}

func (s *Server) runCodexOAuthProtocolLoginStages(ctx context.Context, input codexOAuthActionInput, session *pb.CodexOAuthBrowserSession, data map[string]any) (string, int64, string, error) {
	stepInput := pb.CodexOAuthBrowserStepInput{JobId: input.JobID, AccountId: input.AccountID, Label: input.Label, Session: session}
	stage, issuedAfter, failedStep, err := s.runCodexOAuthStageActivity(stepCodexOAuthProtocolDetect, data, func() (pb.CodexOAuthBrowserStageOutput, error) {
		return s.activities.CodexOAuthDetectProtocolStageActivity(ctx, stepInput)
	})
	if err != nil {
		return stage, issuedAfter, failedStep, err
	}
	if stage == "email" || stage == "" {
		stage, issuedAfter, failedStep, err = s.runCodexOAuthStageActivity(stepCodexOAuthProtocolEmail, data, func() (pb.CodexOAuthBrowserStageOutput, error) {
			return s.activities.CodexOAuthSubmitProtocolEmailActivity(ctx, stepInput)
		})
		if err != nil {
			return stage, issuedAfter, failedStep, err
		}
	}
	if stage == "password" {
		stage, issuedAfter, failedStep, err = s.runCodexOAuthStageActivity(stepCodexOAuthProtocolPassword, data, func() (pb.CodexOAuthBrowserStageOutput, error) {
			return s.activities.CodexOAuthSubmitProtocolPasswordActivity(ctx, stepInput)
		})
		if err != nil {
			return stage, issuedAfter, failedStep, err
		}
	}
	if stage == "email_otp" {
		issuedAfter = codexOAuthEmailOTPIssuedAfter(issuedAfter)
		stage, issuedAfter, failedStep, err = s.runCodexOAuthStageActivity(stepCodexOAuthProtocolEmailOTP, data, func() (pb.CodexOAuthBrowserStageOutput, error) {
			return s.activities.CodexOAuthSubmitProtocolEmailOTPActivity(ctx, pb.CodexOAuthSubmitEmailOTPInput{JobId: input.JobID, AccountId: input.AccountID, Label: input.Label, Session: session, IssuedAfterUnix: issuedAfter})
		})
		if err != nil {
			return stage, issuedAfter, failedStep, err
		}
	}
	if stage == "add_phone" {
		if input.AllowAddPhone {
			return stage, issuedAfter, "", nil
		}
		return stage, issuedAfter, stepCodexOAuthProtocolDetect, fmt.Errorf("codex_oauth_add_phone_required")
	}
	if !codexOAuthProtocolReadyStage(stage) {
		return stage, issuedAfter, codexOAuthProtocolStageStepName(stage), fmt.Errorf("codex oauth protocol login stage not ready: %s", stage)
	}
	return stage, issuedAfter, "", nil
}

func (s *Server) runCodexOAuthStageActivity(stepName string, data map[string]any, fn func() (pb.CodexOAuthBrowserStageOutput, error)) (string, int64, string, error) {
	out, err := fn()
	mergeActionData(data, stepName, structMap(out.GetData()))
	if err != nil {
		return out.GetStage(), out.GetEmailOtpIssuedAfterUnix(), stepName, err
	}
	return out.GetStage(), out.GetEmailOtpIssuedAfterUnix(), "", nil
}

func codexOAuthEmailOTPIssuedAfter(issuedAfter int64) int64 {
	if issuedAfter > 0 {
		return issuedAfter
	}
	return time.Now().Add(-time.Second).Unix()
}

func codexOAuthBrowserReadyStage(stage string) bool {
	switch stage {
	case "add_phone", "consent", "callback":
		return true
	default:
		return false
	}
}

func codexOAuthProtocolReadyStage(stage string) bool {
	switch stage {
	case "consent", "callback":
		return true
	default:
		return false
	}
}

func codexOAuthBrowserStageStepName(stage string) string {
	switch stage {
	case "email":
		return stepCodexOAuthBrowserEmail
	case "password":
		return stepCodexOAuthBrowserPassword
	case "email_otp":
		return stepCodexOAuthBrowserEmailOTP
	default:
		return stepCodexOAuthBrowserDetect
	}
}

func codexOAuthProtocolStageStepName(stage string) string {
	switch stage {
	case "email":
		return stepCodexOAuthProtocolEmail
	case "password":
		return stepCodexOAuthProtocolPassword
	case "email_otp":
		return stepCodexOAuthProtocolEmailOTP
	case "add_phone":
		return stepCodexOAuthProtocolAddPhone
	default:
		return stepCodexOAuthProtocolDetect
	}
}

func (run codexOAuthRun) resultData(accountID string) map[string]any {
	data := map[string]any{"account_id": accountID}
	for key, value := range run.data {
		data[key] = value
	}
	if run.authSecretKey != "" {
		data["auth_secret_key"] = run.authSecretKey
	}
	if run.phoneLabel != "" {
		data["phone_label"] = run.phoneLabel
	}
	if run.phoneReuseCount > 0 {
		data["phone_reuse_count"] = run.phoneReuseCount
	}
	if run.phoneReuseLimit > 0 {
		data["phone_reuse_limit"] = run.phoneReuseLimit
	}
	data["add_phone_confirmed"] = run.addPhoneConfirmed
	data["add_phone_required"] = run.addPhoneRequired
	return data
}

func (result codexOAuthBatchResult) data() map[string]any {
	return map[string]any{
		"total_count":           result.TotalCount,
		"success_count":         result.SuccessCount,
		"add_phone_count":       result.AddPhoneCount,
		"direct_oauth_count":    result.DirectOauthCount,
		"stopped_reason":        result.StoppedReason,
		"phone_label":           result.PhoneLabel,
		"phone_reuse_count":     result.PhoneReuseCount,
		"phone_reuse_limit":     result.PhoneReuseLimit,
		"processed_account_ids": result.ProcessedAccountIDs,
	}
}

func mergeCodexOAuthBatchFailureData(batch map[string]any, run map[string]any) map[string]any {
	for key, value := range run {
		batch[key] = value
	}
	return batch
}

func codexOAuthPhoneLeaseRunData(phone *pb.CodexOAuthPhoneLease) map[string]any {
	if phone == nil || strings.TrimSpace(phone.GetActivationId()) == "" {
		return nil
	}
	data := map[string]any{
		"profile_key":           phone.GetProfileKey(),
		"phone_reused":          phone.GetReused(),
		"phone_reuse_count":     phone.GetReuseCount(),
		"phone_reuse_limit":     phone.GetReuseLimit(),
		"phone_expires_at_unix": phone.GetExpiresAtUnix(),
		"phone_activation_id":   phone.GetActivationId(),
		"phone_country_iso2":    phone.GetCountryIso2(),
		"phone_country_code":    phone.GetCountryCallingCode(),
	}
	if strings.TrimSpace(phone.GetCountryIso2()) != "" {
		data["verification_channel"] = "sms"
	}
	if masked := codexOAuthMaskPhone(phone.GetPhoneE164(), phone.GetPhoneNational()); masked != "" {
		data["phone_mask"] = masked
	}
	return data
}

func codexOAuthMaskPhone(e164, national string) string {
	value := strings.TrimSpace(e164)
	if value == "" {
		value = strings.TrimSpace(national)
	}
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= 4 {
		return strings.Repeat("*", len(runes))
	}
	return strings.Repeat("*", len(runes)-4) + string(runes[len(runes)-4:])
}

func codexOAuthBatchStopReason(message string) string {
	if reason := codexOAuthPhoneSupplyStopReason(message); reason != "" {
		return reason
	}
	text := strings.ToLower(message)
	switch {
	case strings.Contains(text, "phone_reuse_exhausted") || strings.Contains(text, "phone_reuse_exceeded") || strings.Contains(text, "maximum number") || strings.Contains(text, "too many"):
		return "phone_reuse_exhausted"
	case strings.Contains(text, "phone_expired"):
		return "phone_expired"
	case strings.Contains(text, "phone_sms_timeout") || strings.Contains(text, "otp not found"):
		return "phone_sms_timeout"
	default:
		return ""
	}
}

func codexOAuthPhoneSupplyStopReason(message string) string {
	text := strings.ToLower(message)
	switch {
	case strings.Contains(text, "sms_error_code_no_number_available") ||
		strings.Contains(text, "no upstream number available") ||
		strings.Contains(text, "no number available"):
		return "phone_no_number_available"
	case strings.Contains(text, "sms_error_code_supply_unavailable") ||
		strings.Contains(text, "supply unavailable"):
		return "phone_supply_unavailable"
	case strings.Contains(text, "sms_error_code_price_limit_exceeded") ||
		strings.Contains(text, "price limit exceeded"):
		return "phone_price_limit_exceeded"
	case strings.Contains(text, "sms_error_code_insufficient_balance") ||
		strings.Contains(text, "insufficient balance"):
		return "phone_insufficient_balance"
	case strings.Contains(text, "sms_error_code_route_not_found") ||
		strings.Contains(text, "route not found") ||
		strings.Contains(text, "no matching route"):
		return "phone_route_not_found"
	default:
		return ""
	}
}

func codexOAuthCleanAcquirePhoneError(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "acquire phone failed"
	}
	if idx := strings.LastIndex(strings.ToLower(message), "sms_error_code_"); idx >= 0 {
		tail := strings.TrimSpace(message[idx:])
		if sep := strings.LastIndex(tail, ":"); sep >= 0 && sep+1 < len(tail) {
			if detail := strings.TrimSpace(tail[sep+1:]); detail != "" {
				return detail
			}
		}
		return tail
	}
	if idx := strings.LastIndex(message, "AcquireNumber:"); idx >= 0 {
		if detail := strings.TrimSpace(message[idx+len("AcquireNumber:"):]); detail != "" {
			return detail
		}
	}
	return message
}

func codexOAuthPhoneRetryReason(message string) string {
	text := strings.ToLower(message)
	switch {
	case strings.Contains(text, "phone_rate_limited") ||
		strings.Contains(text, "too many phone verification requests") ||
		strings.Contains(text, "too many verification requests") ||
		strings.Contains(text, "try again later"):
		return ""
	case strings.Contains(text, "phone_reuse_exhausted") ||
		strings.Contains(text, "phone_reuse_exceeded") ||
		strings.Contains(text, "already linked to the maximum number") ||
		strings.Contains(text, "used too many") ||
		strings.Contains(text, "maximum number") ||
		strings.Contains(text, "too many"):
		return "phone_reuse_exhausted"
	case strings.Contains(text, "phone_rejected") ||
		strings.Contains(text, "phone_otp_input_missing") ||
		strings.Contains(text, "try a different phone") ||
		strings.Contains(text, "try another phone") ||
		strings.Contains(text, "cannot use this phone") ||
		strings.Contains(text, "can't use this phone") ||
		strings.Contains(text, "invalid phone") ||
		strings.Contains(text, "unsupported phone") ||
		strings.Contains(text, "rejected"):
		return "phone_rejected"
	case strings.Contains(text, "phone_expired"):
		return "phone_expired"
	case strings.Contains(text, "phone_sms_timeout") ||
		strings.Contains(text, "sms_error_code_timeout") ||
		strings.Contains(text, "waitforcode") ||
		strings.Contains(text, "otp not found") ||
		strings.Contains(text, "empty code"):
		return "phone_sms_timeout"
	default:
		return ""
	}
}
