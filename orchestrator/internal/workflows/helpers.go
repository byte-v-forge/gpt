package workflows

import (
	"strings"

	"orchestrator/internal/resultdata"
	pb "orchestrator/pb"

	"google.golang.org/protobuf/types/known/structpb"
)

const (
	browserAuthModeRegister = "register"
	browserAuthModeLogin    = "login"
)

func protoDataMap(data *structpb.Struct) map[string]any {
	return resultdata.Map(data)
}

func protoData(data map[string]any) *structpb.Struct {
	return resultdata.Struct(data)
}

func isAccountAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	normalized := strings.ToLower(err.Error())
	normalized = strings.NewReplacer("_", " ", "-", " ", ".", " ", ":", " ").Replace(normalized)
	return strings.Contains(normalized, "user already exist") ||
		strings.Contains(normalized, "account already exist")
}

func registerFailurePolicy(err error) (status string, recoverable bool, retryable bool) {
	if isAccountAlreadyExistsError(err) {
		return statusFailedFinal, false, false
	}
	return statusFailedRetryable, false, true
}

func isOTPWaitNotReceivedError(err error) bool {
	if err == nil {
		return false
	}
	normalized := strings.ToLower(err.Error())
	return strings.Contains(normalized, "otp not received") ||
		strings.Contains(normalized, "otp not found") ||
		strings.Contains(normalized, "waitcode")
}

func isGoPaySignupOTPNotReceived(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "gopay signup otp not received")
}

func normalizeTier(tier string) string {
	return strings.ToLower(strings.TrimSpace(tier))
}

func normalizeGoPayOTPChannel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "wa", "whatsapp", "otp_wa":
		return "wa"
	case "sms", "otp_sms":
		return "sms"
	default:
		return ""
	}
}

func effectiveGoPayOTPChannel(start GoPayAppOTPOutput, requested string) string {
	if channel := normalizeGoPayOTPChannel(requested); channel != "" {
		return channel
	}
	return normalizeGoPayOTPChannel(start.GetOtpChannel())
}

func goPayAddBalanceMethod(addBalance *GoPayAddBalance) string {
	if addBalance == nil {
		return ""
	}
	if addBalance.GetManualTransfer() != nil {
		return "manual_transfer"
	}
	if addBalance.GetEnvelope() != nil {
		return "envelope"
	}
	if addBalance.GetRekberinaja() != nil {
		return "rekberinaja"
	}
	return ""
}

func goPayAddBalanceMethodOptions() []string {
	return []string{"manual_transfer", "envelope", "rekberinaja"}
}

func goPayOTPWaitInput(jobID, stepName string, start GoPayAppOTPOutput, channel string, activationID string, source string) OTPWaitInput {
	input := OTPWaitInput{
		JobId:            jobID,
		StepName:         stepName,
		TimeoutSeconds:   start.GetTimeoutSeconds(),
		IssuedAfterUnix:  start.GetIssuedAfterUnix(),
		OtpParam:         paymentOTPParam,
		SubmittedAtParam: paymentOTPSubmittedAtParam,
	}
	if effectiveGoPayOTPChannel(start, channel) == "sms" {
		input.Target = &pb.OTPWaitInput_Sms{Sms: &pb.OTPWaitSMSTarget{ActivationId: activationID}}
		return input
	}
	if strings.TrimSpace(source) == "" {
		source = goPayLocalSource
	}
	input.Target = &pb.OTPWaitInput_Payment{Payment: &pb.OTPWaitPaymentTarget{Source: source}}
	return input
}

func ensureLogonData(resp *pb.EnsureLogonResponse) map[string]any {
	if resp == nil || ensureLogonResponseEmpty(resp) {
		return nil
	}
	data := map[string]any{
		"ready":                 resp.GetReady(),
		"already_ready":         resp.GetAlreadyReady(),
		"change_phone_complete": resp.GetChangePhoneComplete(),
		"deactivate_complete":   resp.GetDeactivateComplete(),
		"logon_complete":        resp.GetLogonComplete(),
		"signup_complete":       resp.GetSignupComplete(),
		"signup_pin_complete":   resp.GetSignupPinComplete(),
		"account_token_ready":   resp.GetAccountTokenReady(),
	}
	if resp.GetStage() != "" {
		data["stage"] = resp.GetStage()
	}
	if resp.GetPhone() != "" {
		data["phone"] = resp.GetPhone()
	}
	if status := goPayStatusSnapshotData(resp.GetStatusBefore()); status != nil {
		data["status_before"] = status
	}
	if status := goPayStatusSnapshotData(resp.GetStatusAfter()); status != nil {
		data["status_after"] = status
	}
	if resp.GetErrorMessage() != "" {
		data["error_message"] = resp.GetErrorMessage()
	}
	return data
}

func ensureLogonResponseEmpty(resp *pb.EnsureLogonResponse) bool {
	return !resp.GetReady() &&
		resp.GetStage() == "" &&
		resp.GetPhone() == "" &&
		resp.GetStatusBefore() == nil &&
		resp.GetStatusAfter() == nil &&
		!resp.GetAlreadyReady() &&
		!resp.GetChangePhoneComplete() &&
		!resp.GetDeactivateComplete() &&
		!resp.GetLogonComplete() &&
		!resp.GetSignupComplete() &&
		!resp.GetSignupPinComplete() &&
		!resp.GetAccountTokenReady() &&
		resp.GetErrorMessage() == ""
}

func goPayStatusSnapshotData(snapshot *pb.StatusResponse) map[string]any {
	if snapshot == nil || goPayStatusSnapshotEmpty(snapshot) {
		return nil
	}
	data := map[string]any{
		"token_present": snapshot.GetTokenPresent(),
	}
	if snapshot.GetStage() != "" {
		data["stage"] = snapshot.GetStage()
	}
	if snapshot.GetPhone() != "" {
		data["phone"] = snapshot.GetPhone()
	}
	if snapshot.GetDeviceFingerprint() != "" {
		data["device_fingerprint"] = snapshot.GetDeviceFingerprint()
	}
	if snapshot.GetDeactivatedAt() != 0 {
		data["deactivated_at"] = snapshot.GetDeactivatedAt()
	}
	if snapshot.GetErrorMessage() != "" {
		data["error_message"] = snapshot.GetErrorMessage()
	}
	if snapshot.GetBalanceAmount() != 0 || snapshot.GetBalanceCurrency() != "" || snapshot.GetHasMinBalance() {
		data["balance_amount"] = snapshot.GetBalanceAmount()
		data["has_min_balance"] = snapshot.GetHasMinBalance()
		if snapshot.GetBalanceCurrency() != "" {
			data["balance_currency"] = snapshot.GetBalanceCurrency()
		}
	}
	return data
}

func goPayStatusSnapshotEmpty(snapshot *pb.StatusResponse) bool {
	return snapshot.GetStage() == "" &&
		snapshot.GetPhone() == "" &&
		!snapshot.GetTokenPresent() &&
		snapshot.GetDeviceFingerprint() == "" &&
		snapshot.GetDeactivatedAt() == 0 &&
		snapshot.GetErrorMessage() == "" &&
		snapshot.GetBalanceAmount() == 0 &&
		!snapshot.GetHasMinBalance() &&
		snapshot.GetBalanceCurrency() == ""
}
