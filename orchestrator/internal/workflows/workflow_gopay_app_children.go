package workflows

import (
	"fmt"
	"strings"
	"time"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/log"
	"go.temporal.io/sdk/workflow"
)

const (
	goPayAppAuthChildWorkflowName           = "GoPayAppAuthWorkflow"
	goPayAppEnsureTokenChildWorkflowName    = "GoPayAppEnsureTokenWorkflow"
	goPayAppChangePhoneChildWorkflowName    = "GoPayAppChangePhoneWorkflow"
	goPayAppDeactivateChildWorkflowName     = "GoPayAppDeactivateWorkflow"
	goPayAppSignupChildWorkflowName         = "GoPayAppSignupWorkflow"
	goPayAppEnsurePINSetupChildWorkflowName = "GoPayAppEnsurePINSetupWorkflow"
	goPayAppCreatePinChildWorkflowName      = "GoPayAppCreatePinWorkflow"
)

func GoPayAppAuthWorkflow(ctx workflow.Context, input *GoPayAppOTPWorkflowInput) (*GoPayAppStepOutput, error) {
	activityCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(30*time.Minute))
	cancelCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	out, err := runGoPayAppAuth(ctx, activityCtx, cancelCtx, input.GetJobId(), goPayAppOTPOptionsFromChildInput(input))
	return &out, err
}

func GoPayAppEnsureTokenWorkflow(ctx workflow.Context, input *GoPayAppOTPWorkflowInput) (*GoPayAppStepOutput, error) {
	activityCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(30*time.Minute))
	cancelCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	out, err := runGoPayAppEnsureTokenAvailable(ctx, activityCtx, cancelCtx, input.GetJobId(), goPayAppOTPOptionsFromChildInput(input))
	return &out, err
}

func GoPayAppChangePhoneWorkflow(ctx workflow.Context, input *GoPayAppChangePhoneWorkflowInput) (*GoPayAppStepOutput, error) {
	activityCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(30*time.Minute))
	out, err := runGoPayAppChangePhone(ctx, activityCtx, input.GetJobId(), input.GetStateJson(), input.GetPin(), input.GetCountryCode())
	return &out, err
}

func GoPayAppDeactivateWorkflow(ctx workflow.Context, input *GoPayAppDeactivateWorkflowInput) (*GoPayAppStepOutput, error) {
	activityCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(30*time.Minute))
	out, err := runGoPayAppDeactivate(ctx, activityCtx, input.GetJobId(), input.GetActivationId(), input.GetStateJson(), input.GetPin())
	return &out, err
}

func GoPayAppSignupWorkflow(ctx workflow.Context, input *GoPayAppOTPWorkflowInput) (*GoPayAppStepOutput, error) {
	activityCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(30*time.Minute))
	cancelCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	out, err := runGoPayAppSignup(ctx, activityCtx, cancelCtx, input.GetJobId(), goPayAppOTPOptionsFromChildInput(input))
	return &out, err
}

func GoPayAppEnsurePINSetupWorkflow(ctx workflow.Context, input *GoPayAppOTPWorkflowInput) (*GoPayAppStepOutput, error) {
	activityCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(30*time.Minute))
	cancelCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	out, err := runGoPayAppEnsurePINSetup(ctx, activityCtx, cancelCtx, input.GetJobId(), goPayAppOTPOptionsFromChildInput(input))
	return &out, err
}

func GoPayAppCreatePinWorkflow(ctx workflow.Context, input *GoPayAppOTPWorkflowInput) (*GoPayAppStepOutput, error) {
	activityCtx := workflow.WithActivityOptions(ctx, atomicActivityOptions(30*time.Minute))
	cancelCtx := workflow.WithActivityOptions(ctx, retryableActivityOptions(30*time.Second, 5))
	out, err := runGoPayAppEnsurePINSetup(ctx, activityCtx, cancelCtx, input.GetJobId(), goPayAppOTPOptionsFromChildInput(input))
	return &out, err
}

func runGoPayAppAuthChild(ctx workflow.Context, jobID string, opts goPayAppOTPOptions) (*GoPayAppStepOutput, error) {
	out := &GoPayAppStepOutput{}
	err := executeGoPayChildWorkflow(ctx, goPayAppAuthChildWorkflowName, "gopay-auth", GoPayAppAuthWorkflow, goPayAppOTPWorkflowInput(jobID, opts), &out)
	return out, err
}

func runGoPayAppEnsureTokenChild(ctx workflow.Context, jobID string, opts goPayAppOTPOptions) (*GoPayAppStepOutput, error) {
	out := &GoPayAppStepOutput{}
	err := executeGoPayChildWorkflow(ctx, goPayAppEnsureTokenChildWorkflowName, "gopay-ensure-token", GoPayAppEnsureTokenWorkflow, goPayAppOTPWorkflowInput(jobID, opts), &out)
	return out, err
}

func runGoPayAppChangePhoneChild(ctx workflow.Context, jobID string, stateJSON string, opts goPayAppOTPOptions) (*GoPayAppStepOutput, error) {
	out := &GoPayAppStepOutput{}
	err := executeGoPayChildWorkflow(ctx, goPayAppChangePhoneChildWorkflowName, "gopay-change-phone", GoPayAppChangePhoneWorkflow, &GoPayAppChangePhoneWorkflowInput{
		JobId:       jobID,
		StateJson:   stateJSON,
		Pin:         opts.Pin,
		CountryCode: opts.CountryCode,
	}, &out)
	return out, err
}

func runGoPayAppDeactivateChild(ctx workflow.Context, jobID, activationID, stateJSON string, opts goPayAppOTPOptions) (*GoPayAppStepOutput, error) {
	out := &GoPayAppStepOutput{}
	err := executeGoPayChildWorkflow(ctx, goPayAppDeactivateChildWorkflowName, "gopay-deactivate", GoPayAppDeactivateWorkflow, &GoPayAppDeactivateWorkflowInput{
		JobId:        jobID,
		ActivationId: activationID,
		StateJson:    stateJSON,
		Pin:          opts.Pin,
	}, &out)
	return out, err
}

func runGoPayAppSignupChild(ctx workflow.Context, jobID string, opts goPayAppOTPOptions, attempt int) (*GoPayAppStepOutput, error) {
	out := &GoPayAppStepOutput{}
	err := executeGoPayChildWorkflow(ctx, goPayAppSignupChildWorkflowName, childWorkflowSuffix("gopay-signup", attempt), GoPayAppSignupWorkflow, goPayAppOTPWorkflowInput(jobID, opts), &out)
	return out, err
}

func runGoPayAppEnsurePINSetupChild(ctx workflow.Context, jobID string, opts goPayAppOTPOptions) (*GoPayAppStepOutput, error) {
	out := &GoPayAppStepOutput{}
	err := executeGoPayChildWorkflow(ctx, goPayAppEnsurePINSetupChildWorkflowName, "gopay-ensure-pin-setup", GoPayAppEnsurePINSetupWorkflow, goPayAppOTPWorkflowInput(jobID, opts), &out)
	return out, err
}

func runGoPayAppCreatePinChild(ctx workflow.Context, jobID string, opts goPayAppOTPOptions) (*GoPayAppStepOutput, error) {
	out := &GoPayAppStepOutput{}
	err := executeGoPayChildWorkflow(ctx, goPayAppCreatePinChildWorkflowName, "gopay-create-pin", GoPayAppCreatePinWorkflow, goPayAppOTPWorkflowInput(jobID, opts), &out)
	return out, err
}

func executeGoPayChildWorkflow(ctx workflow.Context, workflowName string, suffix string, workflowDefinition any, input any, output any) error {
	childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
		WorkflowID:        childWorkflowID(ctx, suffix),
		ParentClosePolicy: enumspb.PARENT_CLOSE_POLICY_REQUEST_CANCEL,
	})
	future := workflow.ExecuteChildWorkflow(childCtx, workflowDefinition, input)
	var child workflow.Execution
	if err := future.GetChildWorkflowExecution().Get(ctx, &child); err != nil {
		return err
	}

	logger := workflow.GetLogger(ctx)
	otpSignals := workflow.GetSignalChannel(ctx, manualOTPSignalName)
	addBalanceSignals := workflow.GetSignalChannel(ctx, manualAddBalanceSignalName)
	addBalanceSelectionSignals := workflow.GetSignalChannel(ctx, goPayAddBalanceSelectionSignalName)

	for {
		var (
			done                bool
			childErr            error
			manualOTP           *ManualOTPSignal
			manualAddBalance    *ManualAddBalanceSignal
			addBalanceSelection *ManualAddBalanceSignal
		)
		selector := workflow.NewSelector(ctx)
		selector.AddFuture(future, func(f workflow.Future) {
			childErr = f.Get(ctx, output)
			done = true
		})
		selector.AddReceive(otpSignals, func(c workflow.ReceiveChannel, more bool) {
			var signal ManualOTPSignal
			c.Receive(ctx, &signal)
			manualOTP = &signal
		})
		selector.AddReceive(addBalanceSignals, func(c workflow.ReceiveChannel, more bool) {
			var signal ManualAddBalanceSignal
			c.Receive(ctx, &signal)
			manualAddBalance = &signal
		})
		selector.AddReceive(addBalanceSelectionSignals, func(c workflow.ReceiveChannel, more bool) {
			var signal ManualAddBalanceSignal
			c.Receive(ctx, &signal)
			addBalanceSelection = &signal
		})
		selector.Select(ctx)

		if done {
			return childErr
		}
		if manualOTP != nil {
			forwardChildSignal(ctx, logger, child, workflowName, manualOTPSignalName, manualOTP)
		}
		if manualAddBalance != nil {
			forwardChildSignal(ctx, logger, child, workflowName, manualAddBalanceSignalName, manualAddBalance)
		}
		if addBalanceSelection != nil {
			forwardChildSignal(ctx, logger, child, workflowName, goPayAddBalanceSelectionSignalName, addBalanceSelection)
		}
	}
}

func forwardChildSignal(ctx workflow.Context, logger log.Logger, child workflow.Execution, workflowName string, signalName string, signal any) {
	if err := workflow.SignalExternalWorkflow(ctx, child.ID, child.RunID, signalName, signal).Get(ctx, nil); err != nil {
		logger.Warn("failed to forward signal to child workflow", "workflow", workflowName, "signal", signalName, "child_workflow_id", child.ID, "error", err)
	}
}

func goPayAppOTPWorkflowInput(jobID string, opts goPayAppOTPOptions) *GoPayAppOTPWorkflowInput {
	return &GoPayAppOTPWorkflowInput{
		JobId:           jobID,
		Phone:           opts.Phone,
		OtpChannel:      opts.OTPChannel,
		SmsActivationId: opts.SMSActivationID,
		Source:          opts.Source,
		ResetState:      opts.ResetState,
		StateJson:       opts.StateJSON,
		Pin:             opts.Pin,
		CountryCode:     opts.CountryCode,
		SkipPhoneProbe:  opts.SkipPhoneProbe,
	}
}

func goPayAppOTPOptionsFromChildInput(input *GoPayAppOTPWorkflowInput) goPayAppOTPOptions {
	return goPayAppOTPOptions{
		Phone:           input.GetPhone(),
		OTPChannel:      input.GetOtpChannel(),
		SMSActivationID: input.GetSmsActivationId(),
		Source:          input.GetSource(),
		ResetState:      input.GetResetState(),
		StateJSON:       input.GetStateJson(),
		Pin:             input.GetPin(),
		CountryCode:     input.GetCountryCode(),
		SkipPhoneProbe:  input.GetSkipPhoneProbe(),
	}
}

func childWorkflowID(ctx workflow.Context, suffix string) string {
	parent := workflow.GetInfo(ctx).WorkflowExecution.ID
	return strings.Trim(parent+"-"+suffix, "-")
}

func childWorkflowSuffix(base string, attempt int) string {
	if attempt <= 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, attempt)
}
