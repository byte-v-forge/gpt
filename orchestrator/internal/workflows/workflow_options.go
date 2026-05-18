package workflows

import (
	"fmt"
	"orchestrator/internal/otpwait"
	"strconv"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	otpWaitChannelEmail   = otpwait.ChannelEmail
	otpWaitChannelPayment = otpwait.ChannelPayment
	otpWaitChannelSMS     = otpwait.ChannelSMS

	defaultOTPWaitSeconds = int32(180)
)

func otpWaitInputChannel(input OTPWaitInput) string {
	return otpwait.Channel(&input)
}

func waitForOTP(ctx workflow.Context, input OTPWaitInput) (OTPWaitOutput, error) {
	channel := otpWaitInputChannel(input)
	if channel == "" {
		return OTPWaitOutput{}, fmt.Errorf("otp wait target missing")
	}
	timeoutSeconds := input.GetTimeoutSeconds()
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultOTPWaitSeconds
	}
	input.TimeoutSeconds = timeoutSeconds
	timeout := time.Duration(timeoutSeconds) * time.Second
	waitCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: timeout + 10*time.Second,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 1,
		},
	})
	if channel == otpWaitChannelSMS {
		var out OTPWaitOutput
		err := workflow.ExecuteActivity(waitCtx, waitOTPActivityName, input).Get(ctx, &out)
		return out, err
	}

	manualCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 3))
	otpCtx, cancelOTP := workflow.WithCancel(waitCtx)
	defer cancelOTP()

	otpFuture := workflow.ExecuteActivity(otpCtx, waitOTPActivityName, input)
	timer := workflow.NewTimer(ctx, timeout)
	signalCh := workflow.GetSignalChannel(ctx, manualOTPSignalName)

	otpDone := false
	lastErr := ""
	for {
		var (
			found        bool
			timedOut     bool
			manualSignal bool
			otp          OTPWaitOutput
		)
		selector := workflow.NewSelector(ctx)
		if !otpDone {
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
		selector.AddReceive(signalCh, func(c workflow.ReceiveChannel, more bool) {
			var signal ManualOTPSignal
			c.Receive(ctx, &signal)
			manualSignal = true
		})
		selector.AddFuture(timer, func(f workflow.Future) {
			timedOut = true
		})
		selector.Select(ctx)

		if found {
			cancelOTP()
			return otp, nil
		}
		if manualSignal {
			var manual OTPWaitOutput
			err := workflow.ExecuteActivity(manualCtx, fetchManualOTPActivityName, input).Get(ctx, &manual)
			if err != nil {
				lastErr = err.Error()
				continue
			}
			if manual.GetFound() {
				cancelOTP()
				return manual, nil
			}
		}
		if timedOut {
			cancelOTP()
			if channel == otpWaitChannelPayment {
				if lastErr != "" {
					return OTPWaitOutput{}, fmt.Errorf("payment otp not received after %ds: %s", timeoutSeconds, lastErr)
				}
				return OTPWaitOutput{}, fmt.Errorf("payment otp not received after %ds", timeoutSeconds)
			}
			if lastErr != "" {
				return OTPWaitOutput{}, fmt.Errorf("otp not received after %ds: %s", timeoutSeconds, lastErr)
			}
			return OTPWaitOutput{}, fmt.Errorf("otp not received after %ds", timeoutSeconds)
		}
	}
}

func waitForOTPInStep(ctx workflow.Context, activityCtx workflow.Context, stepName string, input OTPWaitInput) (OTPWaitOutput, error) {
	input.StepName = stepName
	if err := workflow.ExecuteActivity(activityCtx, startJobStepActivityName, JobStepStartInput{
		JobId:       input.GetJobId(),
		StepName:    stepName,
		Recoverable: false,
		Retryable:   true,
		Detail:      protoData(otpWaitStepData(input)),
	}).Get(ctx, nil); err != nil {
		return OTPWaitOutput{}, err
	}

	out, err := waitForOTP(ctx, input)
	if err != nil {
		return out, err
	}
	if err := workflow.ExecuteActivity(activityCtx, completeJobStepActivityName, JobStepCompleteInput{
		JobId:       input.GetJobId(),
		StepName:    stepName,
		Recoverable: false,
		Retryable:   true,
		Result:      protoData(otpWaitStepResultData(input, out)),
	}).Get(ctx, nil); err != nil {
		return out, err
	}
	return out, nil
}

func otpWaitStepData(input OTPWaitInput) map[string]any {
	data := map[string]any{
		"channel":           otpWaitInputChannel(input),
		"timeout_seconds":   input.GetTimeoutSeconds(),
		"issued_after_unix": input.GetIssuedAfterUnix(),
	}
	if target := input.GetEmail(); target != nil {
		data["email"] = target.GetEmail()
	}
	if target := input.GetPayment(); target != nil {
		data["payment_source"] = target.GetSource()
		data["payment_purpose"] = target.GetPurpose()
	}
	if target := input.GetSms(); target != nil {
		data["activation_id"] = target.GetActivationId()
	}
	return data
}

func otpWaitStepResultData(input OTPWaitInput, output OTPWaitOutput) map[string]any {
	data := otpWaitStepData(input)
	for key, value := range protoDataMap(output.GetData()) {
		data[key] = value
	}
	data["found"] = output.GetFound()
	if source := output.GetSource(); source != "" {
		data["source"] = source
	}
	if activationID := output.GetActivationId(); activationID != "" {
		data["activation_id"] = activationID
	}
	if message := output.GetErrorMessage(); message != "" {
		data["error_message"] = message
	}
	return data
}

func waitForManualAddBalance(ctx workflow.Context, timeoutSeconds int32) error {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 1800
	}
	timerCtx, cancelTimer := workflow.WithCancel(ctx)
	timer := workflow.NewTimer(timerCtx, time.Duration(timeoutSeconds)*time.Second)
	signalCh := workflow.GetSignalChannel(ctx, manualAddBalanceSignalName)

	var confirmed bool
	var timedOut bool
	selector := workflow.NewSelector(ctx)
	selector.AddReceive(signalCh, func(c workflow.ReceiveChannel, more bool) {
		var signal ManualAddBalanceSignal
		c.Receive(ctx, &signal)
		confirmed = true
	})
	selector.AddFuture(timer, func(f workflow.Future) {
		timedOut = true
	})
	selector.Select(ctx)

	if confirmed {
		cancelTimer()
		return nil
	}
	if timedOut {
		return fmt.Errorf("manual add_balance not confirmed after %ds", timeoutSeconds)
	}
	return fmt.Errorf("manual add_balance wait ended unexpectedly")
}

func waitForGoPayAddBalanceSelection(ctx workflow.Context, activityCtx workflow.Context, jobID string, timeoutSeconds int32) (*GoPayAddBalance, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 1800
	}
	if err := workflow.ExecuteActivity(activityCtx, startJobStepActivityName, JobStepStartInput{
		JobId:       jobID,
		StepName:    stepGoPayAppAddBalance,
		Recoverable: false,
		Retryable:   true,
		Detail: protoData(map[string]any{
			"status":  "awaiting_selection",
			"methods": goPayAddBalanceMethodOptions(),
		}),
	}).Get(ctx, nil); err != nil {
		return nil, err
	}

	timer := workflow.NewTimer(ctx, time.Duration(timeoutSeconds)*time.Second)
	signalCh := workflow.GetSignalChannel(ctx, goPayAddBalanceSelectionSignalName)
	for {
		var selected *GoPayAddBalance
		var timedOut bool
		selector := workflow.NewSelector(ctx)
		selector.AddReceive(signalCh, func(c workflow.ReceiveChannel, more bool) {
			var signal ManualAddBalanceSignal
			c.Receive(ctx, &signal)
			selected = signal.GetAddBalance()
		})
		selector.AddFuture(timer, func(f workflow.Future) {
			timedOut = true
		})
		selector.Select(ctx)

		if timedOut {
			return nil, fmt.Errorf("add_balance method not selected after %ds", timeoutSeconds)
		}
		if goPayAddBalanceMethod(selected) != "" {
			return selected, nil
		}
	}
}

func atomicActivityOptions(timeout time.Duration) workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout: timeout,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 1,
		},
	}
}
func paymentActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout: 15 * time.Minute,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 1,
		},
	}
}
func heartbeatingActivityOptions(timeout time.Duration, heartbeatTimeout time.Duration) workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout: timeout,
		HeartbeatTimeout:    heartbeatTimeout,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 1,
		},
	}
}
func retryableActivityOptions(timeout time.Duration, attempts int32) workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout: timeout,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    10 * time.Second,
			MaximumAttempts:    attempts,
		},
	}
}
func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
func int32String(value int32) string {
	if value == 0 {
		return ""
	}
	return strconv.FormatInt(int64(value), 10)
}
