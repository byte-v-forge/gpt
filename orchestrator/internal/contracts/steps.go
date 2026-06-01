package contracts

import "github.com/byte-v-forge/gpt/pkg/gptplugin"

const (
	StepRegisterAccount                 = "register_account"
	StepRegisterAccountStart            = "register_account_start"
	StepRegisterAccountBrowser          = "register_account_browser"
	StepProtocolUseProxy                = gptplugin.StepProtocolUseProxy
	StepProtocolAuthEdgeCheck           = gptplugin.StepProtocolAuthEdgeCheck
	StepDynamicIPCreateSession          = gptplugin.StepDynamicIPCreateSession
	StepDynamicIPExitGeo                = gptplugin.StepDynamicIPExitGeo
	StepDynamicIPPreflight              = gptplugin.StepDynamicIPPreflight
	StepRegisterAccountProtocol         = "register_account_protocol"
	StepRegisterAccountProtocolStart    = "register_account_protocol_start"
	StepRegisterAccountProtocolOTPWait  = "register_account_protocol_otp_wait"
	StepRegisterAccountProtocolComplete = "register_account_protocol_complete"
	StepRegisterAccountOTPRequest       = "register_account_otp_request"
	StepRegisterAccountOTPWait          = "register_account_otp_wait"
	StepRegisterAccountComplete         = "register_account_complete"
	StepEnsureLogon                     = gptplugin.StepEnsureLogon
	StepProbePlusTrial                  = gptplugin.StepProbePlusTrial
	StepProbeTier                       = gptplugin.StepProbeTier
	StepLoginSession                    = gptplugin.StepLoginSession
	StepLoginSessionStart               = gptplugin.StepLoginSessionStart
	StepLoginSessionBrowser             = gptplugin.StepLoginSessionBrowser
	StepLoginSessionProtocol            = gptplugin.StepLoginSessionProtocol
	StepLoginSessionProtocolStart       = gptplugin.StepLoginSessionProtocolStart
	StepLoginSessionProtocolOTPWait     = gptplugin.StepLoginSessionProtocolOTPWait
	StepLoginSessionProtocolComplete    = gptplugin.StepLoginSessionProtocolComplete
	StepLoginSessionOTPRequest          = gptplugin.StepLoginSessionOTPRequest
	StepLoginSessionOTPWait             = gptplugin.StepLoginSessionOTPWait
	StepLoginSessionComplete            = gptplugin.StepLoginSessionComplete
	StepCodexOAuthAcquirePhone          = gptplugin.StepCodexOAuthAcquirePhone
	StepCodexOAuthProtocolStart         = gptplugin.StepCodexOAuthProtocolStart
	StepCodexOAuthProtocolDetect        = gptplugin.StepCodexOAuthProtocolDetect
	StepCodexOAuthProtocolEmail         = gptplugin.StepCodexOAuthProtocolEmail
	StepCodexOAuthProtocolPassword      = gptplugin.StepCodexOAuthProtocolPassword
	StepCodexOAuthProtocolEmailOTPWait  = "codex_oauth_protocol_email_otp_wait"
	StepCodexOAuthProtocolEmailOTP      = gptplugin.StepCodexOAuthProtocolEmailOTP
	StepCodexOAuthProtocolAddPhone      = gptplugin.StepCodexOAuthProtocolAddPhone
	StepCodexOAuthProtocolComplete      = gptplugin.StepCodexOAuthProtocolComplete
	StepCodexOAuthBrowserStart          = gptplugin.StepCodexOAuthBrowserStart
	StepCodexOAuthBrowserDetect         = gptplugin.StepCodexOAuthBrowserDetect
	StepCodexOAuthBrowserEmail          = gptplugin.StepCodexOAuthBrowserEmail
	StepCodexOAuthBrowserPassword       = gptplugin.StepCodexOAuthBrowserPassword
	StepCodexOAuthBrowserEmailOTPWait   = "codex_oauth_browser_email_otp_wait"
	StepCodexOAuthBrowserEmailOTP       = gptplugin.StepCodexOAuthBrowserEmailOTP
	StepCodexOAuthBrowserAddPhone       = gptplugin.StepCodexOAuthBrowserAddPhone
	StepCodexOAuthBrowserComplete       = gptplugin.StepCodexOAuthBrowserComplete
	StepCodexOAuthReleasePhone          = gptplugin.StepCodexOAuthReleasePhone
)
