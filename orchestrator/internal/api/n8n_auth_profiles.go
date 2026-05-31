package api

import "orchestrator/internal/channelotpwait"

const (
	protocolRegisterMode = "register"
	protocolLoginMode    = "login"

	n8nRegisterResultSecretPrefix    = "n8n-register-result:"
	n8nRegisterResumeURLSecretPrefix = "n8n-register-resume-url:"

	registerFlowIDParam             = "register_flow_id"
	registerEmailParam              = "register_email"
	registerOTPIssuedAfterParam     = "register_otp_issued_after_unix"
	registerOTPTimeoutParam         = "register_otp_timeout_seconds"
	registerOTPResumeSecretKeyParam = "register_otp_resume_url_secret_key"

	n8nRegisterProtocolResultSecretPrefix    = "n8n-register-protocol-result:"
	n8nRegisterProtocolResumeURLSecretPrefix = "n8n-register-protocol-resume-url:"

	registerProtocolFlowIDParam             = "register_protocol_flow_id"
	registerProtocolEmailParam              = "register_protocol_email"
	registerProtocolOTPIssuedAfterParam     = "register_protocol_otp_issued_after_unix"
	registerProtocolOTPTimeoutParam         = "register_protocol_otp_timeout_seconds"
	registerProtocolOTPResumeSecretKeyParam = "register_protocol_otp_resume_url_secret_key"

	n8nLoginSessionResultSecretPrefix    = "n8n-login-session-result:"
	n8nLoginSessionResumeURLSecretPrefix = "n8n-login-session-resume-url:"

	loginSessionFlowIDParam             = "login_session_flow_id"
	loginSessionEmailParam              = "login_session_email"
	loginSessionOTPIssuedAfterParam     = "login_session_otp_issued_after_unix"
	loginSessionOTPTimeoutParam         = "login_session_otp_timeout_seconds"
	loginSessionOTPResumeSecretKeyParam = "login_session_otp_resume_url_secret_key"

	n8nLoginProtocolResultSecretPrefix    = "n8n-login-session-protocol-result:"
	n8nLoginProtocolResumeURLSecretPrefix = "n8n-login-session-protocol-resume-url:"

	loginProtocolFlowIDParam             = "login_protocol_flow_id"
	loginProtocolEmailParam              = "login_protocol_email"
	loginProtocolOTPIssuedAfterParam     = "login_protocol_otp_issued_after_unix"
	loginProtocolOTPTimeoutParam         = "login_protocol_otp_timeout_seconds"
	loginProtocolOTPResumeSecretKeyParam = "login_protocol_otp_resume_url_secret_key"
)

type n8nBrowserAuthProfile struct {
	Lifecycle n8nBrowserAuthLifecycleConfig
	OTP       n8nChannelOTPWaitConfig
	Finish    n8nAuthFinishConfig
	Fail      n8nAuthFailConfig
}

type n8nProtocolAuthProfile struct {
	Lifecycle n8nProtocolAuthLifecycleConfig
	OTP       n8nChannelOTPWaitConfig
	Finish    n8nAuthFinishConfig
	Fail      n8nAuthFailConfig
}

type n8nRegisterAuthProfile struct {
	Start   n8nRegisterJobConfig
	Browser n8nBrowserAuthProfile
}

type n8nLoginSessionAuthProfile struct {
	Start   n8nActionJobConfig
	Browser n8nBrowserAuthProfile
}

type n8nRegisterProtocolAuthProfile struct {
	Start    n8nRegisterJobConfig
	Protocol n8nProtocolAuthProfile
	Proxy    n8nDynamicProxyProfile
}

type n8nLoginSessionProtocolAuthProfile struct {
	Start    n8nActionJobConfig
	Protocol n8nProtocolAuthProfile
	Proxy    n8nDynamicProxyProfile
}

type n8nAuthOTPDefinition struct {
	StepName             string
	ResumeSecretPrefix   string
	FlowIDParam          string
	IssuedAfterParam     string
	TimeoutParam         string
	ResumeSecretKeyParam string
	PollReason           string
}

func (definition n8nAuthOTPDefinition) waitConfig() n8nChannelOTPWaitConfig {
	return n8nChannelOTPWaitConfig{
		Channel:              channelotpwait.ChannelEmail,
		StepName:             definition.StepName,
		ResumeSecretPrefix:   definition.ResumeSecretPrefix,
		FlowIDParam:          definition.FlowIDParam,
		IssuedAfterParam:     definition.IssuedAfterParam,
		TimeoutParam:         definition.TimeoutParam,
		ResumeSecretKeyParam: definition.ResumeSecretKeyParam,
		PollReason:           definition.PollReason,
	}
}
