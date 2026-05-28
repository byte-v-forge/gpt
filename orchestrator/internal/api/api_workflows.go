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

func (s *Server) RegisterAccount(ctx context.Context, req *pb.RegisterAccountRequest) (*pb.RegisterAccountResponse, error) {
	jobID := uuid.NewString()
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		accountID = uuid.NewString()
	}
	if s.activities == nil {
		return &pb.RegisterAccountResponse{JobId: jobID, ErrorMessage: "GPT action runner is not configured"}, nil
	}
	account, err := s.activities.EnsureAccountActivity(ctx, pb.EnsureAccountInput{Account: &pb.AccountSpec{
		AccountId:     accountID,
		Email:         req.GetEmail(),
		Password:      req.GetPassword(),
		EmailStrategy: requestEmailStrategy(req.GetEmailStrategy()),
		CountryCode:   req.GetCountryCode(),
		Region:        req.GetRegion(),
	}})
	if err != nil {
		return &pb.RegisterAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	accountID = account.GetAccountId()
	if _, err := s.jobStore.CreateWithID(ctx, jobID, accountID, actionRegister, registerAccountJobParams(accountID, req.GetOtpOptions(), req.GetCountryCode(), req.GetRegion())); err != nil {
		return &pb.RegisterAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.RegisterAccountResponse{JobId: jobID, Started: true}, nil
}

func (s *Server) RegisterAccountProtocol(context.Context, *pb.RegisterAccountRequest) (*pb.RegisterAccountResponse, error) {
	return &pb.RegisterAccountResponse{ErrorMessage: "register-protocol is n8n-only; use GPT dashboard BFF workflow endpoint"}, nil
}

func (s *Server) ActivateAccount(ctx context.Context, req *pb.ActivateAccountRequest) (*pb.ActivateAccountResponse, error) {
	jobID := uuid.NewString()
	pinSecretKey, err := s.saveActivationPINSecret(ctx, jobID, req.GetGopayPin())
	if err != nil {
		return &pb.ActivateAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	params := activationJobParams(req, pinSecretKey)
	if _, err := s.jobStore.CreateWithID(ctx, jobID, strings.TrimSpace(req.GetAccountId()), actionActivate, params); err != nil {
		s.deleteRuntimeSecretValue(ctx, pinSecretKey)
		return &pb.ActivateAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.ActivateAccountResponse{JobId: jobID, Started: true}, nil
}

func (s *Server) AutopayAccount(ctx context.Context, req *pb.ActivateAccountRequest) (*pb.ActivateAccountResponse, error) {
	jobID := uuid.NewString()
	pinSecretKey, err := s.saveActivationPINSecret(ctx, jobID, req.GetGopayPin())
	if err != nil {
		return &pb.ActivateAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	params := activationJobParams(req, pinSecretKey)
	if _, err := s.jobStore.CreateWithID(ctx, jobID, strings.TrimSpace(req.GetAccountId()), actionAutopay, params); err != nil {
		s.deleteRuntimeSecretValue(ctx, pinSecretKey)
		return &pb.ActivateAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.ActivateAccountResponse{JobId: jobID, Started: true}, nil
}

func (s *Server) LoginAccount(ctx context.Context, req *pb.LoginAccountRequest) (*pb.LoginAccountResponse, error) {
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		return &pb.LoginAccountResponse{ErrorMessage: "account_id is required"}, nil
	}
	jobID := uuid.NewString()
	if _, err := s.jobStore.CreateWithID(ctx, jobID, accountID, actionLoginSession, map[string]string{"account_id": accountID}); err != nil {
		return &pb.LoginAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.LoginAccountResponse{JobId: jobID, Started: true}, nil
}

func (s *Server) LoginAccountProtocol(ctx context.Context, req *pb.LoginAccountRequest) (*pb.LoginAccountResponse, error) {
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		return &pb.LoginAccountResponse{ErrorMessage: "account_id is required"}, nil
	}
	jobID := uuid.NewString()
	if _, err := s.jobStore.CreateWithID(ctx, jobID, accountID, actionLoginSessionProtocol, map[string]string{"account_id": accountID}); err != nil {
		return &pb.LoginAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.LoginAccountResponse{JobId: jobID, Started: true}, nil
}

func (s *Server) CodexOAuth(ctx context.Context, req *pb.CodexOAuthRequest) (*pb.CodexOAuthResponse, error) {
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		return &pb.CodexOAuthResponse{ErrorMessage: "account_id is required"}, nil
	}
	jobID := uuid.NewString()
	if _, err := s.jobStore.CreateWithID(ctx, jobID, accountID, actionCodexOAuth, codexOAuthJobParams(accountID, req.GetLabel())); err != nil {
		return &pb.CodexOAuthResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.CodexOAuthResponse{JobId: jobID, Started: true}, nil
}

func (s *Server) CodexOAuthProtocol(ctx context.Context, req *pb.CodexOAuthRequest) (*pb.CodexOAuthResponse, error) {
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		return &pb.CodexOAuthResponse{ErrorMessage: "account_id is required"}, nil
	}
	jobID := uuid.NewString()
	if _, err := s.jobStore.CreateWithID(ctx, jobID, accountID, actionCodexOAuthProtocol, codexOAuthJobParams(accountID, req.GetLabel())); err != nil {
		return &pb.CodexOAuthResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.CodexOAuthResponse{JobId: jobID, Started: true}, nil
}

func (s *Server) CodexOAuthAddPhone(ctx context.Context, req *pb.CodexOAuthAddPhoneRequest) (*pb.CodexOAuthAddPhoneResponse, error) {
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		return &pb.CodexOAuthAddPhoneResponse{ErrorMessage: "account_id is required"}, nil
	}
	jobID := uuid.NewString()
	if _, err := s.jobStore.CreateWithID(ctx, jobID, accountID, actionCodexOAuthAddPhone, codexOAuthAddPhoneJobParams(accountID, req.GetLabel(), req.GetMaxReuseCount())); err != nil {
		return &pb.CodexOAuthAddPhoneResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.CodexOAuthAddPhoneResponse{JobId: jobID, Started: true}, nil
}

func (s *Server) CodexOAuthBatchAddPhone(ctx context.Context, req *pb.CodexOAuthBatchAddPhoneRequest) (*pb.CodexOAuthBatchAddPhoneResponse, error) {
	accountIDs := compactAccountIDs(req.GetAccountIds())
	if len(accountIDs) == 0 {
		return &pb.CodexOAuthBatchAddPhoneResponse{ErrorMessage: "account_ids is required"}, nil
	}
	jobID := uuid.NewString()
	if _, err := s.jobStore.CreateWithID(ctx, jobID, "", actionCodexOAuthBatchAddPhone, codexOAuthBatchAddPhoneJobParams(accountIDs, req.GetLabel(), req.GetMaxReuseCount())); err != nil {
		return &pb.CodexOAuthBatchAddPhoneResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.CodexOAuthBatchAddPhoneResponse{JobId: jobID, Started: true}, nil
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

func (s *Server) ProbeAccount(ctx context.Context, req *pb.ProbeAccountRequest) (*pb.ProbeAccountResponse, error) {
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		return &pb.ProbeAccountResponse{ErrorMessage: "account_id is required"}, nil
	}
	jobID := uuid.NewString()
	if _, err := s.jobStore.CreateWithID(ctx, jobID, accountID, actionProbeAccount, map[string]string{"account_id": accountID}); err != nil {
		return &pb.ProbeAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.ProbeAccountResponse{JobId: jobID, Started: true}, nil
}

func (s *Server) RunGoPayApp(ctx context.Context, req *pb.GoPayAppRequest) (*pb.GoPayAppResponse, error) {
	jobID := uuid.NewString()
	pinSecretKey := ""
	if pin := strings.TrimSpace(req.GetPin()); pin != "" {
		pinSecretKey = goPayAppPinSecretKey + jobID
		if err := s.saveRuntimeSecretValue(ctx, pinSecretKey, pin); err != nil {
			return &pb.GoPayAppResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
		}
	}
	params := goPayAppJobParams(req, pinSecretKey)
	if _, err := s.jobStore.CreateWithID(ctx, jobID, "", actionGoPayApp, params); err != nil {
		s.deleteRuntimeSecretValue(ctx, pinSecretKey)
		return &pb.GoPayAppResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.GoPayAppResponse{JobId: jobID, Started: true}, nil
}

func (s *Server) RunGoPayPayment(ctx context.Context, req *pb.GoPayPaymentRequest) (*pb.GoPayPaymentResponse, error) {
	jobID := uuid.NewString()
	otpChannel := strings.TrimSpace(req.GetOtpChannel())
	if otpChannel == "" {
		otpChannel = "sms"
	}
	addBalance := cloneGoPayAddBalance(req.GetAddBalance())
	if addBalance != nil {
		addBalance = s.mergeDefaultGoPayAddBalance(addBalance)
	}
	addBalanceConfirmTimeoutSeconds := req.GetAddBalanceConfirmTimeoutSeconds()
	if addBalanceConfirmTimeoutSeconds <= 0 {
		addBalanceConfirmTimeoutSeconds = s.goPayAddBalanceConfirmTimeoutSeconds
	}
	if addBalanceConfirmTimeoutSeconds <= 0 {
		addBalanceConfirmTimeoutSeconds = 1800
	}
	pinSecretKey := ""
	if pin := strings.TrimSpace(req.GetPin()); pin != "" {
		pinSecretKey = goPayPaymentPinSecretKey + jobID
		if err := s.saveRuntimeSecretValue(ctx, pinSecretKey, pin); err != nil {
			return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
		}
	}
	params, err := goPayPaymentJobParams(req, otpChannel, addBalance, addBalanceConfirmTimeoutSeconds, pinSecretKey)
	if err != nil {
		s.deleteRuntimeSecretValue(ctx, pinSecretKey)
		return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	if _, err := s.jobStore.CreateWithID(ctx, jobID, strings.TrimSpace(req.GetAccountId()), actionGoPayPayment, params); err != nil {
		s.deleteRuntimeSecretValue(ctx, pinSecretKey)
		return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.GoPayPaymentResponse{JobId: jobID, Started: true}, nil
}

func (s *Server) RunGoPayQRISPaymentActivate(ctx context.Context, req *pb.GoPayQRISPaymentActivateRequest) (*pb.GoPayPaymentResponse, error) {
	jobID := uuid.NewString()
	params := goPayQRISPaymentJobParams(req)
	if _, err := s.jobStore.CreateWithID(ctx, jobID, strings.TrimSpace(req.GetAccountId()), actionGoPayQRISPaymentActivate, params); err != nil {
		return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.GoPayPaymentResponse{JobId: jobID, Started: true}, nil
}

func (s *Server) RunGoPayWAPayment(ctx context.Context, req *pb.GoPayWAPaymentRequest) (*pb.GoPayPaymentResponse, error) {
	jobID := uuid.NewString()
	pinSecretKey := ""
	if pin := strings.TrimSpace(req.GetPin()); pin != "" {
		pinSecretKey = goPayWAPaymentPinSecretKey + jobID
		if err := s.saveRuntimeSecretValue(ctx, pinSecretKey, pin); err != nil {
			return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
		}
	}
	accessTokenSecretKey := ""
	if accessToken := strings.TrimSpace(req.GetAccessToken()); accessToken != "" {
		accessTokenSecretKey = goPayWAPaymentAccessTokenSecretKey + jobID
		if err := s.saveRuntimeSecretValue(ctx, accessTokenSecretKey, accessToken); err != nil {
			s.deleteRuntimeSecretValue(ctx, pinSecretKey)
			return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
		}
	}
	params := goPayWAPaymentJobParams(req, pinSecretKey, accessTokenSecretKey)
	if _, err := s.jobStore.CreateWithID(ctx, jobID, strings.TrimSpace(req.GetAccountId()), actionGoPayWAPayment, params); err != nil {
		s.deleteRuntimeSecretValue(ctx, pinSecretKey)
		s.deleteRuntimeSecretValue(ctx, accessTokenSecretKey)
		return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.GoPayPaymentResponse{JobId: jobID, Started: true}, nil
}

func (s *Server) RetryGoPayPaymentRebind(ctx context.Context, req *pb.GoPayPaymentRebindRequest) (*pb.GoPayPaymentResponse, error) {
	sourceJobID := strings.TrimSpace(req.GetSourceJobId())
	if sourceJobID == "" {
		return &pb.GoPayPaymentResponse{ErrorMessage: "source_job_id is required"}, nil
	}
	jobID := uuid.NewString()
	if s.activities == nil {
		return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: "GPT action runner is not configured"}, nil
	}
	source, err := s.activities.GoPayPaymentRebindSourceActivity(ctx, pb.GoPayPaymentRebindSourceInput{
		JobId:       jobID,
		SourceJobId: sourceJobID,
		AccountId:   strings.TrimSpace(req.GetAccountId()),
		UserId:      strings.TrimSpace(req.GetUserId()),
	})
	if err != nil {
		return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	pinSecretKey := ""
	if pin := strings.TrimSpace(req.GetPin()); pin != "" {
		pinSecretKey = goPayPaymentRebindPinSecretKey + jobID
		if err := s.saveRuntimeSecretValue(ctx, pinSecretKey, pin); err != nil {
			return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
		}
	}
	params := goPayPaymentRebindJobParams(source, strings.TrimSpace(req.GetCountryCode()), pinSecretKey)
	if _, err := s.jobStore.CreateWithID(ctx, jobID, source.GetAccountId(), actionGoPayPaymentRebind, params); err != nil {
		s.deleteRuntimeSecretValue(ctx, pinSecretKey)
		return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.GoPayPaymentResponse{JobId: jobID, Started: true}, nil
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
		addBalance := s.mergeDefaultGoPayAddBalance(cloneGoPayAddBalance(req.GetAddBalance()))
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

func cloneGoPayAddBalanceMap(values map[string]*pb.GoPayAddBalance) map[string]*pb.GoPayAddBalance {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]*pb.GoPayAddBalance, len(values))
	for key, value := range values {
		cloned[strings.TrimSpace(key)] = cloneGoPayAddBalance(value)
	}
	return cloned
}

func (s *Server) mergeDefaultGoPayAddBalance(value *pb.GoPayAddBalance) *pb.GoPayAddBalance {
	method := goPayAddBalanceMethod(value)
	if method == "" {
		return cloneGoPayAddBalance(value)
	}
	base := cloneGoPayAddBalance(s.defaultGoPayAddBalances[method])
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
	if value.GetRekberinaja() != nil {
		return "rekberinaja"
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
	case base.GetRekberinaja() != nil && override.GetRekberinaja() != nil:
		mergeRekberinajaAddBalance(base.GetRekberinaja(), override.GetRekberinaja())
	}
}

func mergeRekberinajaAddBalance(dst *pb.GoPayRekberinajaAddBalance, src *pb.GoPayRekberinajaAddBalance) {
	if strings.TrimSpace(src.GetEndpointUrl()) != "" {
		dst.EndpointUrl = src.GetEndpointUrl()
	}
	if strings.TrimSpace(src.GetBearerToken()) != "" {
		dst.BearerToken = src.GetBearerToken()
	}
	if strings.TrimSpace(src.GetRefreshToken()) != "" {
		dst.RefreshToken = src.GetRefreshToken()
	}
	if strings.TrimSpace(src.GetDeviceId()) != "" {
		dst.DeviceId = src.GetDeviceId()
	}
	if strings.TrimSpace(src.GetStore()) != "" {
		dst.Store = src.GetStore()
	}
	if strings.TrimSpace(src.GetProductId()) != "" {
		dst.ProductId = src.GetProductId()
	}
	if strings.TrimSpace(src.GetServiceId()) != "" {
		dst.ServiceId = src.GetServiceId()
	}
	if strings.TrimSpace(src.GetPaymentMethod()) != "" {
		dst.PaymentMethod = src.GetPaymentMethod()
	}
	if strings.TrimSpace(src.GetInvoiceEmail()) != "" {
		dst.InvoiceEmail = src.GetInvoiceEmail()
	}
	if strings.TrimSpace(src.GetPromoCode()) != "" {
		dst.PromoCode = src.GetPromoCode()
	}
	if src.GetUsePoin() {
		dst.UsePoin = true
	}
	if strings.TrimSpace(src.GetUserAgent()) != "" {
		dst.UserAgent = src.GetUserAgent()
	}
	if strings.TrimSpace(src.GetOrigin()) != "" {
		dst.Origin = src.GetOrigin()
	}
	if strings.TrimSpace(src.GetReferer()) != "" {
		dst.Referer = src.GetReferer()
	}
	if src.GetFeeTotal() > 0 {
		dst.FeeTotal = src.GetFeeTotal()
	}
	if src.GetPollTimeoutSeconds() > 0 {
		dst.PollTimeoutSeconds = src.GetPollTimeoutSeconds()
	}
	if src.GetPollIntervalSeconds() > 0 {
		dst.PollIntervalSeconds = src.GetPollIntervalSeconds()
	}
}

func (s *Server) RegisterAndActivateAccount(ctx context.Context, req *pb.RegisterAndActivateAccountRequest) (*pb.RegisterAndActivateAccountResponse, error) {
	jobID := uuid.NewString()
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		accountID = uuid.NewString()
	}
	if s.activities == nil {
		return &pb.RegisterAndActivateAccountResponse{JobId: jobID, ErrorMessage: "GPT action runner is not configured"}, nil
	}
	account, err := s.activities.EnsureAccountActivity(ctx, pb.EnsureAccountInput{Account: &pb.AccountSpec{
		AccountId:     accountID,
		Email:         req.GetEmail(),
		Password:      req.GetPassword(),
		EmailStrategy: requestEmailStrategy(req.GetEmailStrategy()),
		CountryCode:   req.GetCountryCode(),
		Region:        req.GetRegion(),
	}})
	if err != nil {
		return &pb.RegisterAndActivateAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	accountID = account.GetAccountId()
	pinSecretKey, err := s.saveActivationPINSecret(ctx, jobID, req.GetGopayPin())
	if err != nil {
		return &pb.RegisterAndActivateAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	params := registerAndActivateJobParams(accountID, req, pinSecretKey)
	if _, err := s.jobStore.CreateWithID(ctx, jobID, accountID, actionRegisterAndActivate, params); err != nil {
		s.deleteRuntimeSecretValue(ctx, pinSecretKey)
		return &pb.RegisterAndActivateAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.RegisterAndActivateAccountResponse{JobId: jobID, Started: true}, nil
}

func registerAndActivateJobParams(accountID string, req *pb.RegisterAndActivateAccountRequest, pinSecretKey string) map[string]string {
	params := registerAccountJobParams(accountID, req.GetOtpOptions(), req.GetCountryCode(), req.GetRegion())
	if value := strings.TrimSpace(req.GetGopayPhone()); value != "" {
		params["gopay_phone"] = value
	}
	if value := strings.TrimSpace(req.GetGopayCountryCode()); value != "" {
		params["gopay_country_code"] = value
	}
	if pinSecretKey = strings.TrimSpace(pinSecretKey); pinSecretKey != "" {
		params["gopay_pin_secret_key"] = pinSecretKey
	}
	return params
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

func (s *Server) saveActivationPINSecret(ctx context.Context, jobID string, pin string) (string, error) {
	pin = strings.TrimSpace(pin)
	if pin == "" {
		return "", nil
	}
	key := activationGoPayPinSecretKey + jobID
	if err := s.saveRuntimeSecretValue(ctx, key, pin); err != nil {
		return "", err
	}
	return key, nil
}

func activationJobParams(req *pb.ActivateAccountRequest, pinSecretKey string) map[string]string {
	params := map[string]string{}
	if value := strings.TrimSpace(req.GetAccountId()); value != "" {
		params["account_id"] = value
	}
	if value := strings.TrimSpace(req.GetJobId()); value != "" {
		params["source_job_id"] = value
	}
	if value := strings.TrimSpace(req.GetGopayPhone()); value != "" {
		params["gopay_phone"] = value
	}
	if value := strings.TrimSpace(req.GetGopayCountryCode()); value != "" {
		params["gopay_country_code"] = value
	}
	if pinSecretKey = strings.TrimSpace(pinSecretKey); pinSecretKey != "" {
		params["gopay_pin_secret_key"] = pinSecretKey
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
