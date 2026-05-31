package contracts

import "github.com/byte-v-forge/gpt/pkg/gptplugin"

const (
	ActionRegister                = "REGISTER"
	ActionProbeAccount            = gptplugin.ActionProbeAccount
	ActionLoginSession            = gptplugin.ActionLoginSession
	ActionRegisterProtocol        = "REGISTER_PROTOCOL"
	ActionLoginSessionProtocol    = gptplugin.ActionLoginSessionProtocol
	ActionCodexOAuth              = gptplugin.ActionCodexOAuth
	ActionCodexOAuthProtocol      = gptplugin.ActionCodexOAuthProtocol
	ActionCodexOAuthAddPhone      = gptplugin.ActionCodexOAuthAddPhone
	ActionCodexOAuthBatchAddPhone = gptplugin.ActionCodexOAuthBatchAddPhone
)

const (
	CapabilityAccountProbe = gptplugin.CapabilityAccountProbe
	CapabilityBrowserAuth  = gptplugin.CapabilityBrowserAuth
	CapabilityProtocolAuth = gptplugin.CapabilityProtocolAuth
	CapabilityRegistration = gptplugin.CapabilityRegistration
	CapabilityActivation   = gptplugin.CapabilityActivation
	CapabilityPayment      = gptplugin.CapabilityPayment
	CapabilityLogin        = gptplugin.CapabilityLogin
	CapabilityCodexOAuth   = gptplugin.CapabilityCodexOAuth
	CapabilityPhoneBinding = gptplugin.CapabilityPhoneBinding
	CapabilityN8NWorkflow  = gptplugin.CapabilityN8NWorkflow
)
