package api

import (
	"orchestrator/internal/contracts"
	"orchestrator/internal/jobstatus"
)

type n8nCodexOAuthProfile struct {
	Start            n8nActionJobConfig
	Proxy            n8nDynamicProxyProfile
	OTP              n8nChannelOTPWaitConfig
	Complete         n8nActionSuccessConfig
	Fail             n8nActionFailureStoreConfig
	IncludeAccountID bool
}

type n8nCodexOAuthProfileDefinition struct {
	ActionID            string
	Flow                n8nCodexOAuthFlowKind
	ProxyMode           string
	AuthEdgeCheckTarget string
	FailureStepFallback string
	FailureStatus       string
	FailureRecoverable  bool
	FailureRetryable    bool
	IncludeAccountID    bool
}

var n8nCodexOAuthProfileDefinitions = []n8nCodexOAuthProfileDefinition{
	{
		ActionID:            contracts.ActionCodexOAuth,
		Flow:                n8nCodexOAuthBrowserFlow,
		FailureStepFallback: contracts.StepCodexOAuthBrowserStart,
		FailureStatus:       jobstatus.FailedRecoverable,
		FailureRecoverable:  true,
		IncludeAccountID:    true,
	},
	{
		ActionID:            contracts.ActionCodexOAuthProtocol,
		Flow:                n8nCodexOAuthProtocolFlow,
		ProxyMode:           codexOAuthProtocolMode,
		AuthEdgeCheckTarget: n8nAuthEdgeCheckTargetCSRF,
		FailureStepFallback: contracts.StepCodexOAuthProtocolStart,
		FailureStatus:       jobstatus.FailedRecoverable,
		FailureRecoverable:  true,
		IncludeAccountID:    true,
	},
	{
		ActionID:            contracts.ActionCodexOAuthAddPhone,
		Flow:                n8nCodexOAuthProtocolFlow,
		ProxyMode:           codexOAuthProtocolAddPhoneMode,
		AuthEdgeCheckTarget: n8nAuthEdgeCheckTargetCSRF,
		FailureStepFallback: contracts.StepCodexOAuthProtocolStart,
		FailureStatus:       jobstatus.FailedRetryable,
		FailureRetryable:    true,
		IncludeAccountID:    true,
	},
	{
		ActionID:            contracts.ActionCodexOAuthBatchAddPhone,
		FailureStepFallback: "codex_oauth_batch_add_phone",
		FailureStatus:       jobstatus.FailedRetryable,
		FailureRetryable:    true,
	},
}

func n8nCodexOAuthProfileForAction(actionID string) (n8nCodexOAuthProfile, error) {
	definition, ok := n8nCodexOAuthProfileDefinitionForAction(actionID)
	if !ok {
		return n8nCodexOAuthProfile{}, unsupportedN8NAuthActionError("codex oauth", contracts.NormalizeActionID(actionID))
	}
	return definition.profile(), nil
}

func n8nCodexOAuthProfileDefinitionForAction(actionID string) (n8nCodexOAuthProfileDefinition, bool) {
	normalized := contracts.NormalizeActionID(actionID)
	for _, definition := range n8nCodexOAuthProfileDefinitions {
		if contracts.NormalizeActionID(definition.ActionID) == normalized {
			return definition, true
		}
	}
	return n8nCodexOAuthProfileDefinition{}, false
}

func (definition n8nCodexOAuthProfileDefinition) profile() n8nCodexOAuthProfile {
	action := contracts.ResolveActionProfile(definition.ActionID)
	profile := n8nCodexOAuthProfile{
		Start:    (n8nActionJobConfig{}).withAction(action),
		Complete: (n8nActionSuccessConfig{}).withAction(action),
		Fail: (n8nActionFailureStoreConfig{
			FailureStepFallback: definition.FailureStepFallback,
			Status:              definition.FailureStatus,
			Recoverable:         definition.FailureRecoverable,
			Retryable:           definition.FailureRetryable,
		}).withAction(action),
		IncludeAccountID: definition.IncludeAccountID,
	}
	profile.OTP = definition.emailOTPWaitConfig()
	if definition.ProxyMode != "" || definition.AuthEdgeCheckTarget != "" {
		profile.Proxy = (n8nDynamicProxyProfile{
			ProtocolMode:        definition.ProxyMode,
			AuthEdgeCheckTarget: definition.AuthEdgeCheckTarget,
		}).withAction(action)
	}
	return profile
}

func (definition n8nCodexOAuthProfileDefinition) runtimeProfile() n8nActionRuntimeProfile {
	profile := definition.profile()
	runtime := n8nActionRuntimeProfile{
		CodexOAuth:     n8nRuntimeProfilePtr(profile),
		CodexOAuthFlow: definition.Flow,
	}
	if profile.Proxy.Purpose != "" {
		runtime.DynamicProxy = n8nRuntimeProfilePtr(profile.Proxy)
	}
	return runtime
}

func (definition n8nCodexOAuthProfileDefinition) emailOTPWaitConfig() n8nChannelOTPWaitConfig {
	switch definition.Flow {
	case n8nCodexOAuthBrowserFlow:
		return n8nAuthOTPDefinition{
			StepName:             contracts.StepCodexOAuthBrowserEmailOTPWait,
			ResumeSecretPrefix:   "n8n-codex-oauth-browser-email-otp-resume-url:",
			FlowIDParam:          "codex_oauth_browser_flow_id",
			IssuedAfterParam:     "codex_oauth_email_otp_issued_after_unix",
			TimeoutParam:         "codex_oauth_email_otp_timeout_seconds",
			ResumeSecretKeyParam: "codex_oauth_email_otp_resume_url_secret_key",
			PollReason:           "codex_oauth_browser_email_otp_wait",
		}.waitConfig()
	case n8nCodexOAuthProtocolFlow:
		return n8nAuthOTPDefinition{
			StepName:             contracts.StepCodexOAuthProtocolEmailOTPWait,
			ResumeSecretPrefix:   "n8n-codex-oauth-protocol-email-otp-resume-url:",
			FlowIDParam:          "codex_oauth_protocol_flow_id",
			IssuedAfterParam:     "codex_oauth_email_otp_issued_after_unix",
			TimeoutParam:         "codex_oauth_email_otp_timeout_seconds",
			ResumeSecretKeyParam: "codex_oauth_email_otp_resume_url_secret_key",
			PollReason:           "codex_oauth_protocol_email_otp_wait",
		}.waitConfig()
	default:
		return n8nChannelOTPWaitConfig{}
	}
}
