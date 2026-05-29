package api

import (
	"context"
	"strconv"
	"strings"

	"github.com/google/uuid"
	proto "google.golang.org/protobuf/proto"

	"orchestrator/internal/accountfingerprint"
	"orchestrator/internal/activities"
	"orchestrator/pb"
)

func (s *Server) CreateGPTAccount(ctx context.Context, req *pb.CreateGPTAccountRequest) (*pb.CreateGPTAccountResponse, error) {
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		accountID = uuid.NewString()
	}
	email := strings.TrimSpace(req.GetEmail())
	if email == "" {
		allocated, err := activities.NewAccountEmailAllocator(s.accountClient).Allocate(ctx, accountID, nil, requestEmailStrategy(req.GetEmailStrategy()))
		if err != nil {
			return &pb.CreateGPTAccountResponse{ErrorMessage: err.Error()}, nil
		}
		email = strings.TrimSpace(allocated)
	}
	if email == "" {
		return &pb.CreateGPTAccountResponse{ErrorMessage: "email allocator returned empty email"}, nil
	}

	resp, err := s.accountClient.CreateAccount(ctx, &pb.CreateAccountRequest{Account: &pb.Account{
		AccountId: accountID,
		Email:     email,
		Password:  req.GetPassword(),
	}})
	if err != nil {
		return &pb.CreateGPTAccountResponse{ErrorMessage: err.Error()}, nil
	}
	if err := s.generateAccountFingerprint(ctx, resp.GetAccount().GetAccountId(), accountfingerprint.GenerateParams{
		CountryCode: req.GetCountryCode(),
		Region:      req.GetRegion(),
	}); err != nil {
		return &pb.CreateGPTAccountResponse{ErrorMessage: err.Error()}, nil
	}
	return &pb.CreateGPTAccountResponse{Account: resp.GetAccount()}, nil
}

func (s *Server) RegisterAccount(context.Context, *pb.RegisterAccountRequest) (*pb.RegisterAccountResponse, error) {
	return &pb.RegisterAccountResponse{ErrorMessage: "register is n8n-only; use GPT dashboard BFF workflow endpoint"}, nil
}

func (s *Server) RegisterAccountProtocol(context.Context, *pb.RegisterAccountRequest) (*pb.RegisterAccountResponse, error) {
	return &pb.RegisterAccountResponse{ErrorMessage: "register-protocol is n8n-only; use GPT dashboard BFF workflow endpoint"}, nil
}

func (s *Server) LoginAccount(context.Context, *pb.LoginAccountRequest) (*pb.LoginAccountResponse, error) {
	return &pb.LoginAccountResponse{ErrorMessage: "login-session is n8n-only; use GPT dashboard BFF workflow endpoint"}, nil
}

func (s *Server) LoginAccountProtocol(context.Context, *pb.LoginAccountRequest) (*pb.LoginAccountResponse, error) {
	return &pb.LoginAccountResponse{ErrorMessage: "login-session-protocol is n8n-only; use GPT dashboard BFF workflow endpoint"}, nil
}

func (s *Server) CodexOAuth(context.Context, *pb.CodexOAuthRequest) (*pb.CodexOAuthResponse, error) {
	return &pb.CodexOAuthResponse{ErrorMessage: "codex-oauth is n8n-only; use GPT dashboard BFF workflow endpoint"}, nil
}

func (s *Server) CodexOAuthProtocol(context.Context, *pb.CodexOAuthRequest) (*pb.CodexOAuthResponse, error) {
	return &pb.CodexOAuthResponse{ErrorMessage: "codex-oauth-protocol is n8n-only; use GPT dashboard BFF workflow endpoint"}, nil
}

func (s *Server) CodexOAuthAddPhone(context.Context, *pb.CodexOAuthAddPhoneRequest) (*pb.CodexOAuthAddPhoneResponse, error) {
	return &pb.CodexOAuthAddPhoneResponse{ErrorMessage: "codex-oauth-add-phone is n8n-only; use GPT dashboard BFF workflow endpoint"}, nil
}

func (s *Server) CodexOAuthBatchAddPhone(context.Context, *pb.CodexOAuthBatchAddPhoneRequest) (*pb.CodexOAuthBatchAddPhoneResponse, error) {
	return &pb.CodexOAuthBatchAddPhoneResponse{ErrorMessage: "codex-oauth-batch-add-phone is n8n-only; use GPT dashboard BFF workflow endpoint"}, nil
}

func compactAccountIDs(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		accountID := strings.TrimSpace(value)
		if accountID == "" || seen[accountID] {
			continue
		}
		seen[accountID] = true
		out = append(out, accountID)
	}
	return out
}

func (s *Server) ProbeAccount(context.Context, *pb.ProbeAccountRequest) (*pb.ProbeAccountResponse, error) {
	return &pb.ProbeAccountResponse{ErrorMessage: "probe-account is n8n-only; use GPT dashboard BFF workflow endpoint"}, nil
}

func (s *Server) RunGoPayApp(context.Context, *pb.GoPayAppRequest) (*pb.GoPayAppResponse, error) {
	return &pb.GoPayAppResponse{ErrorMessage: "gopay-app is n8n-only; use GPT dashboard BFF workflow endpoint"}, nil
}

func (s *Server) RunGoPayPayment(context.Context, *pb.GoPayPaymentRequest) (*pb.GoPayPaymentResponse, error) {
	return &pb.GoPayPaymentResponse{ErrorMessage: "gopay-payment is n8n-only; use GPT dashboard BFF workflow endpoint"}, nil
}

func (s *Server) RunGoPayQRISPaymentActivate(context.Context, *pb.GoPayQRISPaymentActivateRequest) (*pb.GoPayPaymentResponse, error) {
	return &pb.GoPayPaymentResponse{ErrorMessage: "gopay-qris-payment-activate is n8n-only; use GPT dashboard BFF workflow endpoint"}, nil
}

func (s *Server) RunGoPayWAPayment(context.Context, *pb.GoPayWAPaymentRequest) (*pb.GoPayPaymentResponse, error) {
	return &pb.GoPayPaymentResponse{ErrorMessage: "gopay-wa-payment is n8n-only; use GPT dashboard BFF workflow endpoint"}, nil
}

func (s *Server) RetryGoPayPaymentRebind(context.Context, *pb.GoPayPaymentRebindRequest) (*pb.GoPayPaymentResponse, error) {
	return &pb.GoPayPaymentResponse{ErrorMessage: "gopay-payment-rebind is n8n-only; use GPT dashboard BFF workflow endpoint"}, nil
}

func (s *Server) ConfirmManualAddBalance(ctx context.Context, req *pb.ConfirmManualAddBalanceRequest) (*pb.ConfirmManualAddBalanceResponse, error) {
	jobID := strings.TrimSpace(req.GetJobId())
	if jobID == "" {
		return &pb.ConfirmManualAddBalanceResponse{Success: false, ErrorMessage: "job_id is required"}, nil
	}
	job, err := s.getJob(ctx, jobID)
	if err != nil {
		return &pb.ConfirmManualAddBalanceResponse{Success: false, JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	if job.Status != statusRunning {
		return &pb.ConfirmManualAddBalanceResponse{Success: false, JobId: jobID, ErrorMessage: "job is not running: " + job.Status}, nil
	}
	if job.Action != actionGoPayPayment {
		return &pb.ConfirmManualAddBalanceResponse{Success: false, JobId: jobID, ErrorMessage: "job does not accept add_balance confirmation: " + job.Action}, nil
	}
	if job.LastStep != stepGoPayAppEnsureBalance && job.LastStep != stepGoPayAppEnsureBalanceConfirm {
		return &pb.ConfirmManualAddBalanceResponse{Success: false, JobId: jobID, ErrorMessage: "job is not waiting for ensure_balance confirmation: " + job.LastStep}, nil
	}
	if req.GetAddBalance() != nil {
		addBalance := s.mergeDefaultGoPayAddBalance(ctx, cloneGoPayAddBalance(req.GetAddBalance()))
		if goPayAddBalanceMethod(addBalance) == "" {
			return &pb.ConfirmManualAddBalanceResponse{Success: false, JobId: jobID, ErrorMessage: "add_balance method is required"}, nil
		}
		encoded, err := encodeGoPayAddBalance(addBalance)
		if err != nil {
			return &pb.ConfirmManualAddBalanceResponse{Success: false, JobId: jobID, ErrorMessage: err.Error()}, nil
		}
		if err := s.setJobParams(ctx, jobID, map[string]string{goPayAddBalanceSelectionParam: encoded}); err != nil {
			return &pb.ConfirmManualAddBalanceResponse{Success: false, JobId: jobID, ErrorMessage: err.Error()}, nil
		}
		return &pb.ConfirmManualAddBalanceResponse{Success: true, JobId: jobID}, nil
	}
	if err := s.setJobParams(ctx, jobID, map[string]string{manualAddBalanceConfirmParam: "true"}); err != nil {
		return &pb.ConfirmManualAddBalanceResponse{Success: false, JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.ConfirmManualAddBalanceResponse{Success: true, JobId: jobID}, nil
}

func (s *Server) ConfirmManualGoPayPayment(ctx context.Context, req *pb.ConfirmManualGoPayPaymentRequest) (*pb.ConfirmManualGoPayPaymentResponse, error) {
	jobID := strings.TrimSpace(req.GetJobId())
	if jobID == "" {
		return &pb.ConfirmManualGoPayPaymentResponse{Success: false, ErrorMessage: "job_id is required"}, nil
	}
	job, err := s.getJob(ctx, jobID)
	if err != nil {
		return &pb.ConfirmManualGoPayPaymentResponse{Success: false, JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	if job.Status != statusRunning {
		return &pb.ConfirmManualGoPayPaymentResponse{Success: false, JobId: jobID, ErrorMessage: "job is not running: " + job.Status}, nil
	}
	if job.Action != actionGoPayQRISPaymentActivate && job.Action != actionGoPayPayment {
		return &pb.ConfirmManualGoPayPaymentResponse{Success: false, JobId: jobID, ErrorMessage: "job does not accept manual gopay payment confirmation: " + job.Action}, nil
	}
	if job.LastStep != stepGoPayPayment {
		return &pb.ConfirmManualGoPayPaymentResponse{Success: false, JobId: jobID, ErrorMessage: "job is not waiting for gopay payment confirmation: " + job.LastStep}, nil
	}
	if err := s.setJobParams(ctx, jobID, map[string]string{manualGoPayPaymentConfirmParam: "true"}); err != nil {
		return &pb.ConfirmManualGoPayPaymentResponse{Success: false, JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.ConfirmManualGoPayPaymentResponse{Success: true, JobId: jobID}, nil
}

func cloneGoPayAddBalance(value *pb.GoPayAddBalance) *pb.GoPayAddBalance {
	if value == nil {
		return nil
	}
	cloned, ok := proto.Clone(value).(*pb.GoPayAddBalance)
	if !ok {
		return nil
	}
	return cloned
}

func (s *Server) mergeDefaultGoPayAddBalance(ctx context.Context, value *pb.GoPayAddBalance) *pb.GoPayAddBalance {
	method := goPayAddBalanceMethod(value)
	if method == "" {
		return cloneGoPayAddBalance(value)
	}
	base := s.configuredGoPayAddBalanceByMethod(ctx, method)
	if base == nil {
		return cloneGoPayAddBalance(value)
	}
	mergeGoPayAddBalance(base, value)
	return base
}

func goPayAddBalanceMethod(value *pb.GoPayAddBalance) string {
	if value == nil {
		return ""
	}
	if value.GetManualTransfer() != nil {
		return "manual_transfer"
	}
	if value.GetEnvelope() != nil {
		return "envelope"
	}
	return ""
}

func mergeGoPayAddBalance(base *pb.GoPayAddBalance, override *pb.GoPayAddBalance) {
	switch {
	case base.GetManualTransfer() != nil && override.GetManualTransfer() != nil:
		dst := base.GetManualTransfer()
		src := override.GetManualTransfer()
		if strings.TrimSpace(src.GetInstructions()) != "" {
			dst.Instructions = src.GetInstructions()
		}
		if src.GetAmount() > 0 {
			dst.Amount = src.GetAmount()
		}
		if strings.TrimSpace(src.GetCurrency()) != "" {
			dst.Currency = src.GetCurrency()
		}
	case base.GetEnvelope() != nil && override.GetEnvelope() != nil:
		dst := base.GetEnvelope()
		src := override.GetEnvelope()
		if strings.TrimSpace(src.GetLink()) != "" {
			dst.Link = src.GetLink()
		}
		if strings.TrimSpace(src.GetEnvelopeRequestId()) != "" {
			dst.EnvelopeRequestId = src.GetEnvelopeRequestId()
		}
	}
}

func registerAccountJobParams(accountID string, options *pb.RegisterOTPOptions, countryCode string, region string) map[string]string {
	params := map[string]string{"account_id": accountID}
	putProtocolGeoParams(params, countryCode, region)
	if options == nil {
		return params
	}
	params["registration_otp_mode"] = options.GetMode().String()
	if options.AutoResend != nil {
		params["registration_otp_auto_resend"] = boolString(options.GetAutoResend())
	}
	if options.GetFirstWaitSeconds() > 0 {
		params["registration_otp_first_wait_seconds"] = int32String(options.GetFirstWaitSeconds())
	}
	if options.GetTimeoutSeconds() > 0 {
		params["registration_otp_timeout_seconds"] = int32String(options.GetTimeoutSeconds())
	}
	return params
}

func codexOAuthJobParams(accountID string, label string) map[string]string {
	params := map[string]string{"account_id": strings.TrimSpace(accountID)}
	if label = strings.TrimSpace(label); label != "" {
		params["label"] = label
	}
	return params
}

func codexOAuthAddPhoneJobParams(accountID string, label string, maxReuseCount int32) map[string]string {
	params := codexOAuthJobParams(accountID, label)
	if maxReuseCount > 0 {
		params["max_reuse_count"] = int32String(maxReuseCount)
	}
	return params
}

func codexOAuthBatchAddPhoneJobParams(accountIDs []string, label string, maxReuseCount int32) map[string]string {
	params := map[string]string{
		"account_ids":   strings.Join(compactAccountIDs(accountIDs), ","),
		"account_count": int32String(int32(len(accountIDs))),
	}
	if label = strings.TrimSpace(label); label != "" {
		params["label"] = label
	}
	if maxReuseCount > 0 {
		params["max_reuse_count"] = int32String(maxReuseCount)
	}
	return params
}

func goPayQRISPaymentJobParams(req *pb.GoPayQRISPaymentActivateRequest) map[string]string {
	params := map[string]string{
		"activation_mode":       "qris_payment",
		"payment_type":          "qris",
		"tokenization":          "qris",
		"otp_channel":           "not_required",
		"uses_wa":               "false",
		"uses_gopay_app_flow":   "false",
		"manual_confirmation":   "true",
		"manual_payment_button": "true",
	}
	if value := strings.TrimSpace(req.GetAccountId()); value != "" {
		params["account_id"] = value
	}
	if value := strings.TrimSpace(req.GetSourceJobId()); value != "" {
		params["source_job_id"] = value
	}
	return params
}

func goPayPaymentJobParams(req *pb.GoPayPaymentRequest, otpChannel string, addBalance *pb.GoPayAddBalance, timeoutSeconds int32, pinSecretKey string) (map[string]string, error) {
	params := map[string]string{
		"otp_channel":                         strings.TrimSpace(otpChannel),
		"add_balance_confirm_timeout_seconds": int32String(timeoutSeconds),
	}
	if value := strings.TrimSpace(req.GetAccountId()); value != "" {
		params["account_id"] = value
	}
	if value := strings.TrimSpace(req.GetSourceJobId()); value != "" {
		params["source_job_id"] = value
	}
	if value := strings.TrimSpace(req.GetSmsActivationId()); value != "" {
		params["sms_activation_id"] = value
	}
	if value := strings.TrimSpace(req.GetUserId()); value != "" {
		params["user_id"] = value
	}
	if value := strings.TrimSpace(req.GetWaPhone()); value != "" {
		params["wa_phone"] = value
	}
	if value := strings.TrimSpace(req.GetCountryCode()); value != "" {
		params["country_code"] = value
	}
	if pinSecretKey = strings.TrimSpace(pinSecretKey); pinSecretKey != "" {
		params["pin_secret_key"] = pinSecretKey
	}
	if addBalance != nil {
		encoded, err := encodeGoPayAddBalance(addBalance)
		if err != nil {
			return nil, err
		}
		params[goPayPaymentAddBalanceParam] = encoded
		params["add_balance_method"] = goPayAddBalanceMethod(addBalance)
	}
	return params, nil
}

func goPayWAPaymentJobParams(req *pb.GoPayWAPaymentRequest, pinSecretKey string, accessTokenSecretKey string) map[string]string {
	params := map[string]string{
		"otp_channel":  "wa",
		"payment_only": "true",
	}
	if value := strings.TrimSpace(req.GetAccountId()); value != "" {
		params["account_id"] = value
	}
	if value := strings.TrimSpace(req.GetSourceJobId()); value != "" {
		params["source_job_id"] = value
	}
	if value := strings.TrimSpace(req.GetUserId()); value != "" {
		params["user_id"] = value
	}
	if value := strings.TrimSpace(req.GetWaPhone()); value != "" {
		params["wa_phone"] = value
	}
	if value := strings.TrimSpace(req.GetCountryCode()); value != "" {
		params["country_code"] = value
	}
	if pinSecretKey = strings.TrimSpace(pinSecretKey); pinSecretKey != "" {
		params["pin_secret_key"] = pinSecretKey
	}
	if accessTokenSecretKey = strings.TrimSpace(accessTokenSecretKey); accessTokenSecretKey != "" {
		params["access_token_secret_key"] = accessTokenSecretKey
	}
	return params
}

func goPayPaymentRebindJobParams(source pb.GoPayPaymentRebindSourceOutput, countryCode string, pinSecretKey string) map[string]string {
	params := map[string]string{
		"source_job_id": source.GetSourceJobId(),
		"account_id":    source.GetAccountId(),
		"user_id":       source.GetUserId(),
		"wa_phone":      source.GetWaPhone(),
	}
	if countryCode = strings.TrimSpace(countryCode); countryCode != "" {
		params["country_code"] = countryCode
	}
	if pinSecretKey = strings.TrimSpace(pinSecretKey); pinSecretKey != "" {
		params["pin_secret_key"] = pinSecretKey
	}
	return params
}

func goPayAppJobParams(req *pb.GoPayAppRequest, pinSecretKey string) map[string]string {
	params := map[string]string{
		"operation": normalizeGoPayAppOperation(req.GetOperation()),
	}
	if value := strings.TrimSpace(req.GetPhone()); value != "" {
		params["phone"] = value
	}
	if value := strings.TrimSpace(req.GetCountryCode()); value != "" {
		params["country_code"] = value
	}
	if value := strings.TrimSpace(req.GetOtpChannel()); value != "" {
		params["otp_channel"] = value
	}
	if value := strings.TrimSpace(req.GetUserId()); value != "" {
		params["user_id"] = value
	}
	if pinSecretKey = strings.TrimSpace(pinSecretKey); pinSecretKey != "" {
		params["pin_secret_key"] = pinSecretKey
	}
	return params
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func int32String(value int32) string {
	return strconv.FormatInt(int64(value), 10)
}

func requestEmailStrategy(strategy pb.AccountEmailStrategy) pb.AccountEmailStrategy {
	if strategy == pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_UNSPECIFIED {
		return pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_POOLED_ALIAS
	}
	return strategy
}
