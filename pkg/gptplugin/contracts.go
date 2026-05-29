package gptplugin

const (
	ActionProbeAccount            = "PROBE_ACCOUNT"
	ActionLoginSession            = "LOGIN_SESSION"
	ActionLoginSessionProtocol    = "LOGIN_SESSION_PROTOCOL"
	ActionCodexOAuth              = "CODEX_OAUTH"
	ActionCodexOAuthProtocol      = "CODEX_OAUTH_PROTOCOL"
	ActionCodexOAuthAddPhone      = "CODEX_OAUTH_ADD_PHONE"
	ActionCodexOAuthBatchAddPhone = "CODEX_OAUTH_BATCH_ADD_PHONE"
)

const (
	CapabilityAccountProbe = "account_probe"
	CapabilityBrowserAuth  = "browser_auth"
	CapabilityProtocolAuth = "protocol_auth"
	CapabilityRegistration = "registration"
	CapabilityActivation   = "activation"
	CapabilityPayment      = "payment"
	CapabilityLogin        = "login"
	CapabilityCodexOAuth   = "codex_oauth"
	CapabilityPhoneBinding = "phone_binding"
	CapabilityN8NWorkflow  = "n8n_workflow"
)
