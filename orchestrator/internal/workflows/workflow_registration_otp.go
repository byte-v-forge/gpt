package workflows

import (
	"fmt"
	"time"

	pb "orchestrator/pb"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const defaultRegistrationOTPFirstWaitSeconds = int32(30)

type registrationOTPWaitOptions struct {
	JobID            string
	AccountID        string
	FlowID           string
	Email            string
	IssuedAfterUnix  int64
	TimeoutSeconds   int32
	Policy           *pb.RegisterOTPOptions
	OtpParam         string
	SubmittedAtParam string
}

type registrationOTPPolicy struct {
	ManualOnly       bool
	AutoResend       bool
	FirstWaitSeconds int32
	TimeoutSeconds   int32
	Mode             string
}

type registrationOTPWaitRound struct {
	OTP             OTPWaitOutput
	Resend          BrowserAuthResendOTPOutput
	Found           bool
	ResendRequested bool
	TimedOut        bool
	Error           error
}

func waitForBrowserRegistrationOTP(ctx workflow.Context, activityCtx workflow.Context, browserCtx workflow.Context, opts registrationOTPWaitOptions) (OTPWaitOutput, int64, error) {
	policy := effectiveRegistrationOTPPolicy(opts.Policy, opts.TimeoutSeconds)
	waitInput := OTPWaitInput{
		JobId:            opts.JobID,
		StepName:         stepRegisterAccountOTPWait,
		Target:           &pb.OTPWaitInput_Email{Email: &pb.OTPWaitEmailTarget{Email: opts.Email}},
		TimeoutSeconds:   policy.TimeoutSeconds,
		IssuedAfterUnix:  opts.IssuedAfterUnix,
		OtpParam:         opts.OtpParam,
		SubmittedAtParam: opts.SubmittedAtParam,
	}
	if err := workflow.ExecuteActivity(activityCtx, startJobStepActivityName, JobStepStartInput{
		JobId:       opts.JobID,
		StepName:    stepRegisterAccountOTPWait,
		Recoverable: false,
		Retryable:   true,
		Detail:      protoData(registrationOTPWaitStepData(waitInput, policy, 0)),
	}).Get(ctx, nil); err != nil {
		return OTPWaitOutput{}, waitInput.GetIssuedAfterUnix(), err
	}

	resendCount := 0
	autoResent := false
	for {
		timeoutSeconds := policy.TimeoutSeconds
		if policy.AutoResend && !autoResent && !policy.ManualOnly {
			timeoutSeconds = policy.FirstWaitSeconds
		}
		round := waitRegistrationOTPRound(ctx, activityCtx, browserCtx, waitInput, policy, timeoutSeconds, opts.AccountID, opts.FlowID)
		if round.Found {
			if err := completeRegistrationOTPWaitStep(ctx, activityCtx, opts.JobID, waitInput, policy, resendCount, round.OTP); err != nil {
				return round.OTP, waitInput.GetIssuedAfterUnix(), err
			}
			return round.OTP, waitInput.GetIssuedAfterUnix(), nil
		}
		if round.ResendRequested {
			resendCount++
			waitInput.IssuedAfterUnix = round.Resend.GetOtpIssuedAfterUnix()
			waitInput.TimeoutSeconds = policy.TimeoutSeconds
			continue
		}
		if round.TimedOut && policy.AutoResend && !autoResent && !policy.ManualOnly {
			autoResent = true
			manual, err := fetchManualRegistrationOTP(ctx, activityCtx, waitInput)
			if err != nil {
				return manual, waitInput.GetIssuedAfterUnix(), err
			}
			if manual.GetFound() {
				if err := completeRegistrationOTPWaitStep(ctx, activityCtx, opts.JobID, waitInput, policy, resendCount, manual); err != nil {
					return manual, waitInput.GetIssuedAfterUnix(), err
				}
				return manual, waitInput.GetIssuedAfterUnix(), nil
			}
			resend, err := resendBrowserRegistrationOTP(ctx, browserCtx, opts.JobID, opts.AccountID, opts.FlowID)
			if err != nil {
				return OTPWaitOutput{}, waitInput.GetIssuedAfterUnix(), err
			}
			resendCount++
			waitInput.IssuedAfterUnix = resend.GetOtpIssuedAfterUnix()
			waitInput.TimeoutSeconds = policy.TimeoutSeconds
			continue
		}
		if round.Error != nil {
			return round.OTP, waitInput.GetIssuedAfterUnix(), round.Error
		}
		if round.TimedOut {
			return round.OTP, waitInput.GetIssuedAfterUnix(), fmt.Errorf("otp not received after %ds", timeoutSeconds)
		}
		return round.OTP, waitInput.GetIssuedAfterUnix(), fmt.Errorf("otp wait ended unexpectedly")
	}
}

func completeRegistrationOTPWaitStep(ctx workflow.Context, activityCtx workflow.Context, jobID string, input OTPWaitInput, policy registrationOTPPolicy, resendCount int, output OTPWaitOutput) error {
	result := otpWaitStepResultData(input, output)
	for key, value := range registrationOTPWaitStepData(input, policy, resendCount) {
		result[key] = value
	}
	return workflow.ExecuteActivity(activityCtx, completeJobStepActivityName, JobStepCompleteInput{
		JobId:       jobID,
		StepName:    stepRegisterAccountOTPWait,
		Recoverable: false,
		Retryable:   true,
		Result:      protoData(result),
	}).Get(ctx, nil)
}

func waitRegistrationOTPRound(ctx workflow.Context, activityCtx workflow.Context, browserCtx workflow.Context, input OTPWaitInput, policy registrationOTPPolicy, timeoutSeconds int32, accountID string, flowID string) registrationOTPWaitRound {
	input.TimeoutSeconds = timeoutSeconds
	if manual, err := fetchManualRegistrationOTP(ctx, activityCtx, input); err != nil {
		return registrationOTPWaitRound{Error: err}
	} else if manual.GetFound() {
		return registrationOTPWaitRound{OTP: manual, Found: true}
	}

	timeout := time.Duration(timeoutSeconds) * time.Second
	manualCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 3))
	timerCtx, cancelTimer := workflow.WithCancel(ctx)
	timer := workflow.NewTimer(timerCtx, timeout)
	manualSignalCh := workflow.GetSignalChannel(ctx, manualOTPSignalName)
	resendSignalCh := workflow.GetSignalChannel(ctx, otpResendSignalName)

	var (
		otpFuture workflow.Future
		cancelOTP workflow.CancelFunc
		otpDone   bool
		lastErr   string
	)
	if !policy.ManualOnly {
		waitCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
			StartToCloseTimeout: timeout + 10*time.Second,
			HeartbeatTimeout:    30 * time.Second,
			RetryPolicy: &temporal.RetryPolicy{
				MaximumAttempts: 1,
			},
		})
		otpCtx, cancel := workflow.WithCancel(waitCtx)
		cancelOTP = cancel
		otpFuture = workflow.ExecuteActivity(otpCtx, waitOTPActivityName, input)
	}
	cancelWaiting := func() {
		cancelTimer()
		if cancelOTP != nil {
			cancelOTP()
		}
	}

	for {
		var (
			found           bool
			timedOut        bool
			manualSignal    bool
			resendRequested bool
			otp             OTPWaitOutput
		)
		selector := workflow.NewSelector(ctx)
		if otpFuture != nil && !otpDone {
			selector.AddFuture(otpFuture, func(f workflow.Future) {
				var out OTPWaitOutput
				if err := f.Get(ctx, &out); err != nil {
					lastErr = err.Error()
				} else if out.GetFound() {
					otp = out
					found = true
				}
				otpDone = true
			})
		}
		selector.AddReceive(manualSignalCh, func(c workflow.ReceiveChannel, more bool) {
			var signal ManualOTPSignal
			c.Receive(ctx, &signal)
			manualSignal = true
		})
		selector.AddReceive(resendSignalCh, func(c workflow.ReceiveChannel, more bool) {
			var signal OTPResendSignal
			c.Receive(ctx, &signal)
			resendRequested = true
		})
		selector.AddFuture(timer, func(f workflow.Future) {
			timedOut = true
		})
		selector.Select(ctx)

		if found {
			cancelWaiting()
			return registrationOTPWaitRound{OTP: otp, Found: true}
		}
		if manualSignal {
			var manual OTPWaitOutput
			err := workflow.ExecuteActivity(manualCtx, fetchManualOTPActivityName, input).Get(ctx, &manual)
			if err != nil {
				lastErr = err.Error()
				continue
			}
			if manual.GetFound() {
				cancelWaiting()
				return registrationOTPWaitRound{OTP: manual, Found: true}
			}
		}
		if resendRequested {
			cancelWaiting()
			resend, err := resendBrowserRegistrationOTP(ctx, browserCtx, input.GetJobId(), accountID, flowID)
			if err != nil {
				return registrationOTPWaitRound{Error: err}
			}
			return registrationOTPWaitRound{Resend: resend, ResendRequested: true}
		}
		if timedOut {
			cancelWaiting()
			if lastErr != "" {
				return registrationOTPWaitRound{TimedOut: true, Error: fmt.Errorf("otp not received after %ds: %s", timeoutSeconds, lastErr)}
			}
			return registrationOTPWaitRound{TimedOut: true}
		}
	}
}

func fetchManualRegistrationOTP(ctx workflow.Context, activityCtx workflow.Context, input OTPWaitInput) (OTPWaitOutput, error) {
	var manual OTPWaitOutput
	err := workflow.ExecuteActivity(activityCtx, fetchManualOTPActivityName, input).Get(ctx, &manual)
	return manual, err
}

func resendBrowserRegistrationOTP(ctx workflow.Context, browserCtx workflow.Context, jobID string, accountID string, flowID string) (BrowserAuthResendOTPOutput, error) {
	var resend BrowserAuthResendOTPOutput
	err := workflow.ExecuteActivity(browserCtx, browserAuthResendOTPActivityName, BrowserAuthResendOTPInput{
		JobId:     jobID,
		AccountId: accountID,
		FlowId:    flowID,
		Mode:      browserAuthModeRegister,
	}).Get(ctx, &resend)
	if err != nil {
		return resend, err
	}
	if !resend.GetSuccess() {
		return resend, fmt.Errorf("browser register OTP resend failed: %s", resend.GetErrorMessage())
	}
	if resend.GetOtpIssuedAfterUnix() <= 0 {
		return resend, fmt.Errorf("browser register OTP resend returned empty issued_after timestamp")
	}
	return resend, nil
}

func effectiveRegistrationOTPPolicy(options *pb.RegisterOTPOptions, fallbackTimeout int32) registrationOTPPolicy {
	mode := "auto"
	manualOnly := false
	if options.GetMode() == pb.RegisterOTPMode_REGISTER_OTP_MODE_MANUAL {
		mode = "manual"
		manualOnly = true
	}
	timeoutSeconds := options.GetTimeoutSeconds()
	if timeoutSeconds <= 0 {
		timeoutSeconds = fallbackTimeout
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultOTPWaitSeconds
	}
	firstWaitSeconds := options.GetFirstWaitSeconds()
	if firstWaitSeconds <= 0 {
		firstWaitSeconds = defaultRegistrationOTPFirstWaitSeconds
	}
	autoResend := !manualOnly
	if options != nil && options.AutoResend != nil {
		autoResend = options.GetAutoResend()
	}
	return registrationOTPPolicy{
		ManualOnly:       manualOnly,
		AutoResend:       autoResend,
		FirstWaitSeconds: firstWaitSeconds,
		TimeoutSeconds:   timeoutSeconds,
		Mode:             mode,
	}
}

func registrationOTPWaitStepData(input OTPWaitInput, policy registrationOTPPolicy, resendCount int) map[string]any {
	data := otpWaitStepData(input)
	data["otp_mode"] = policy.Mode
	data["manual_only"] = policy.ManualOnly
	data["auto_resend"] = policy.AutoResend
	data["first_wait_seconds"] = policy.FirstWaitSeconds
	data["resend_count"] = resendCount
	return data
}

func registrationEmailOTPWaitInput(jobID string, email string, timeoutSeconds int32, issuedAfterUnix int64) OTPWaitInput {
	return OTPWaitInput{
		JobId:            jobID,
		StepName:         stepRegisterAccountOTPWait,
		Target:           &pb.OTPWaitInput_Email{Email: &pb.OTPWaitEmailTarget{Email: email}},
		TimeoutSeconds:   timeoutSeconds,
		IssuedAfterUnix:  issuedAfterUnix,
		OtpParam:         registrationOTPParam,
		SubmittedAtParam: registrationOTPSubmittedAtParam,
	}
}
