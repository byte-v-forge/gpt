package contracts

import "strings"

const (
	ActionRegister                 = "REGISTER"
	ActionActivate                 = "ACTIVATE"
	ActionAutopay                  = "AUTOPAY"
	ActionGoPayApp                 = "GOPAY_APP"
	ActionGoPayPayment             = "GOPAY_PAYMENT"
	ActionGoPayQRISPaymentActivate = "GOPAY_QRIS_PAYMENT_ACTIVATE"
	ActionGoPayWAPayment           = "GOPAY_WA_PAYMENT"
	ActionGoPayPaymentRebind       = "GOPAY_PAYMENT_REBIND"
	ActionProbeAccount             = "PROBE_ACCOUNT"
	ActionLoginSession             = "LOGIN_SESSION"
	ActionRegisterProtocol         = "REGISTER_PROTOCOL"
	ActionLoginSessionProtocol     = "LOGIN_SESSION_PROTOCOL"
	ActionCodexOAuth               = "CODEX_OAUTH"
	ActionCodexOAuthProtocol       = "CODEX_OAUTH_PROTOCOL"
	ActionCodexOAuthAddPhone       = "CODEX_OAUTH_ADD_PHONE"
	ActionCodexOAuthBatchAddPhone  = "CODEX_OAUTH_BATCH_ADD_PHONE"
	ActionRegisterAndActivate      = "REGISTER_AND_ACTIVATE"
)

const (
	ManualAddBalanceConfirmationParam   = "manual_add_balance_confirmed"
	GoPayAddBalanceSelectionParam       = "gopay_add_balance_selected"
	ManualGoPayPaymentConfirmationParam = "manual_gopay_payment_confirmed"
)

func WorkflowID(action string, jobID string) (string, bool) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return "", false
	}
	switch strings.TrimSpace(action) {
	case ActionRegister:
		return "register-" + jobID, true
	case ActionActivate:
		return "activate-" + jobID, true
	case ActionAutopay:
		return "autopay-" + jobID, true
	case ActionGoPayApp:
		return "gopay-app-" + jobID, true
	case ActionGoPayPayment:
		return "gopay-payment-" + jobID, true
	case ActionGoPayQRISPaymentActivate:
		return "gopay-qris-payment-activate-" + jobID, true
	case ActionGoPayWAPayment:
		return "gopay-wa-payment-" + jobID, true
	case ActionGoPayPaymentRebind:
		return "gopay-payment-rebind-" + jobID, true
	case ActionProbeAccount:
		return "probe-" + jobID, true
	case ActionLoginSession:
		return "login-session-" + jobID, true
	case ActionRegisterProtocol:
		return "register-protocol-" + jobID, true
	case ActionLoginSessionProtocol:
		return "login-session-protocol-" + jobID, true
	case ActionCodexOAuth:
		return "codex-oauth-" + jobID, true
	case ActionCodexOAuthProtocol:
		return "codex-oauth-protocol-" + jobID, true
	case ActionCodexOAuthAddPhone:
		return "codex-oauth-add-phone-" + jobID, true
	case ActionCodexOAuthBatchAddPhone:
		return "codex-oauth-batch-add-phone-" + jobID, true
	case ActionRegisterAndActivate:
		return "register-activate-" + jobID, true
	default:
		return "", false
	}
}
