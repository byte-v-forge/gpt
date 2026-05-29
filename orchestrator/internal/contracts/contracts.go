package contracts

import "github.com/byte-v-forge/gpt/pkg/gptplugin"

const (
	ActionRegister                 = "REGISTER"
	ActionGoPayApp                 = "GOPAY_APP"
	ActionGoPayPayment             = "GOPAY_PAYMENT"
	ActionGoPayQRISPaymentActivate = "GOPAY_QRIS_PAYMENT_ACTIVATE"
	ActionGoPayWAPayment           = "GOPAY_WA_PAYMENT"
	ActionGoPayPaymentRebind       = "GOPAY_PAYMENT_REBIND"
	ActionProbeAccount             = gptplugin.ActionProbeAccount
	ActionLoginSession             = gptplugin.ActionLoginSession
	ActionRegisterProtocol         = "REGISTER_PROTOCOL"
	ActionLoginSessionProtocol     = gptplugin.ActionLoginSessionProtocol
	ActionCodexOAuth               = gptplugin.ActionCodexOAuth
	ActionCodexOAuthProtocol       = gptplugin.ActionCodexOAuthProtocol
	ActionCodexOAuthAddPhone       = gptplugin.ActionCodexOAuthAddPhone
	ActionCodexOAuthBatchAddPhone  = gptplugin.ActionCodexOAuthBatchAddPhone
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
	CapabilityGoPay        = "gopay"
	CapabilityN8NWorkflow  = gptplugin.CapabilityN8NWorkflow
)

const (
	ManualAddBalanceConfirmationParam   = "manual_add_balance_confirmed"
	GoPayAddBalanceSelectionParam       = "gopay_add_balance_selected"
	ManualGoPayPaymentConfirmationParam = "manual_gopay_payment_confirmed"
)
