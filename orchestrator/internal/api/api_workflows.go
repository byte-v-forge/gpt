package api

import (
	"context"
	"github.com/google/uuid"
	"orchestrator/internal/activities"
	"orchestrator/internal/contracts"
	"orchestrator/internal/workflows"
	"orchestrator/pb"
	"strings"

	proto "google.golang.org/protobuf/proto"
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
	return &pb.CreateGPTAccountResponse{Account: resp.GetAccount()}, nil
}

func (s *Server) RegisterAccount(ctx context.Context, req *pb.RegisterAccountRequest) (*pb.RegisterAccountResponse, error) {
	jobID := uuid.NewString()
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		accountID = uuid.NewString()
	}
	_, err := s.temporal.ExecuteWorkflow(ctx, s.workflowOptions(workflowIDForAction(actionRegister, jobID)), workflows.RegisterAccountWorkflow, workflows.RegisterAccountWorkflowInput{
		JobId: jobID,
		Account: &workflows.AccountSpec{
			AccountId:     accountID,
			Email:         req.GetEmail(),
			Password:      req.GetPassword(),
			EmailStrategy: requestEmailStrategy(req.GetEmailStrategy()),
		},
		OtpOptions: req.GetOtpOptions(),
	})
	if err != nil {
		return nil, err
	}
	return &pb.RegisterAccountResponse{JobId: jobID, Started: true}, nil
}

func (s *Server) ActivateAccount(ctx context.Context, req *pb.ActivateAccountRequest) (*pb.ActivateAccountResponse, error) {
	jobID := uuid.NewString()
	var result workflows.ActivateAccountWorkflowResult
	run, err := s.temporal.ExecuteWorkflow(ctx, s.workflowOptions(workflowIDForAction(actionActivate, jobID)), workflows.ActivateAccountWorkflow, workflows.ActivateAccountWorkflowInput{
		JobId:            jobID,
		AccountId:        strings.TrimSpace(req.GetAccountId()),
		SourceJobId:      req.GetJobId(),
		Action:           actionActivate,
		GopayPhone:       req.GetGopayPhone(),
		GopayCountryCode: req.GetGopayCountryCode(),
		GopayPin:         req.GetGopayPin(),
	})
	if err != nil {
		return nil, err
	}
	if err := run.Get(ctx, &result); err != nil {
		return &pb.ActivateAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}

	return &pb.ActivateAccountResponse{
		JobId:        result.GetJobId(),
		Success:      result.GetSuccess(),
		ErrorMessage: result.GetErrorMessage(),
		ChargeRef:    result.GetChargeRef(),
		SnapToken:    result.GetSnapToken(),
	}, nil
}

func (s *Server) AutopayAccount(ctx context.Context, req *pb.ActivateAccountRequest) (*pb.ActivateAccountResponse, error) {
	jobID := uuid.NewString()
	var result workflows.AutoPayWorkflowResult
	run, err := s.temporal.ExecuteWorkflow(ctx, s.workflowOptions(workflowIDForAction(actionAutopay, jobID)), workflows.AutoPayWorkflow, workflows.AutoPayWorkflowInput{
		JobId:            jobID,
		AccountId:        strings.TrimSpace(req.GetAccountId()),
		SourceJobId:      req.GetJobId(),
		GopayPhone:       req.GetGopayPhone(),
		GopayCountryCode: req.GetGopayCountryCode(),
		GopayPin:         req.GetGopayPin(),
	})
	if err != nil {
		return nil, err
	}
	if err := run.Get(ctx, &result); err != nil {
		return &pb.ActivateAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}

	return &pb.ActivateAccountResponse{
		JobId:        result.GetJobId(),
		Success:      result.GetSuccess(),
		ErrorMessage: result.GetErrorMessage(),
		ChargeRef:    result.GetChargeRef(),
		SnapToken:    result.GetSnapToken(),
	}, nil
}

func (s *Server) LoginAccount(ctx context.Context, req *pb.LoginAccountRequest) (*pb.LoginAccountResponse, error) {
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		return &pb.LoginAccountResponse{ErrorMessage: "account_id is required"}, nil
	}
	jobID := uuid.NewString()
	_, err := s.temporal.ExecuteWorkflow(ctx, s.workflowOptions(workflowIDForAction(actionLoginSession, jobID)), workflows.LoginSessionWorkflow, workflows.LoginSessionWorkflowInput{
		JobId:     jobID,
		AccountId: accountID,
	})
	if err != nil {
		return &pb.LoginAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.LoginAccountResponse{JobId: jobID, Started: true}, nil
}

func (s *Server) ProbeAccount(ctx context.Context, req *pb.ProbeAccountRequest) (*pb.ProbeAccountResponse, error) {
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		return &pb.ProbeAccountResponse{ErrorMessage: "account_id is required"}, nil
	}
	jobID := uuid.NewString()
	_, err := s.temporal.ExecuteWorkflow(ctx, s.workflowOptions(workflowIDForAction(actionProbeAccount, jobID)), workflows.ProbeAccountWorkflow, workflows.ProbeAccountWorkflowInput{
		JobId:     jobID,
		AccountId: accountID,
	})
	if err != nil {
		return &pb.ProbeAccountResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.ProbeAccountResponse{JobId: jobID, Started: true}, nil
}

func (s *Server) RunGoPayApp(ctx context.Context, req *pb.GoPayAppRequest) (*pb.GoPayAppResponse, error) {
	jobID := uuid.NewString()
	_, err := s.temporal.ExecuteWorkflow(ctx, s.workflowOptions(workflowIDForAction(actionGoPayApp, jobID)), workflows.GoPayAppWorkflow, workflows.GoPayAppWorkflowInput{
		JobId:       jobID,
		Phone:       strings.TrimSpace(req.GetPhone()),
		CountryCode: strings.TrimSpace(req.GetCountryCode()),
		Pin:         strings.TrimSpace(req.GetPin()),
		OtpChannel:  strings.TrimSpace(req.GetOtpChannel()),
		UserId:      strings.TrimSpace(req.GetUserId()),
		Operation:   strings.TrimSpace(req.GetOperation()),
	})
	if err != nil {
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
	_, err := s.temporal.ExecuteWorkflow(ctx, s.workflowOptions(workflowIDForAction(actionGoPayPayment, jobID)), workflows.GoPayPaymentWorkflow, workflows.GoPayPaymentWorkflowInput{
		JobId:                           jobID,
		AccountId:                       strings.TrimSpace(req.GetAccountId()),
		SourceJobId:                     strings.TrimSpace(req.GetSourceJobId()),
		OtpChannel:                      otpChannel,
		SmsActivationId:                 strings.TrimSpace(req.GetSmsActivationId()),
		AddBalance:                      addBalance,
		AddBalanceConfirmTimeoutSeconds: addBalanceConfirmTimeoutSeconds,
		UserId:                          strings.TrimSpace(req.GetUserId()),
		WaPhone:                         strings.TrimSpace(req.GetWaPhone()),
		Pin:                             strings.TrimSpace(req.GetPin()),
		CountryCode:                     strings.TrimSpace(req.GetCountryCode()),
	})
	if err != nil {
		return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.GoPayPaymentResponse{JobId: jobID, Started: true}, nil
}

func (s *Server) RunGoPayQRISPaymentActivate(ctx context.Context, req *pb.GoPayQRISPaymentActivateRequest) (*pb.GoPayPaymentResponse, error) {
	jobID := uuid.NewString()
	_, err := s.temporal.ExecuteWorkflow(ctx, s.workflowOptions(workflowIDForAction(actionGoPayQRISPaymentActivate, jobID)), workflows.GoPayQRISPaymentActivateWorkflow, workflows.GoPayPaymentWorkflowInput{
		JobId:       jobID,
		AccountId:   strings.TrimSpace(req.GetAccountId()),
		SourceJobId: strings.TrimSpace(req.GetSourceJobId()),
	})
	if err != nil {
		return &pb.GoPayPaymentResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	return &pb.GoPayPaymentResponse{JobId: jobID, Started: true}, nil
}

func (s *Server) RunGoPayWAPayment(ctx context.Context, req *pb.GoPayWAPaymentRequest) (*pb.GoPayPaymentResponse, error) {
	jobID := uuid.NewString()
	input := workflows.GoPayWAPaymentWorkflowInput{
		JobId:       jobID,
		SourceJobId: strings.TrimSpace(req.GetSourceJobId()),
		UserId:      strings.TrimSpace(req.GetUserId()),
		WaPhone:     strings.TrimSpace(req.GetWaPhone()),
		Pin:         strings.TrimSpace(req.GetPin()),
		CountryCode: strings.TrimSpace(req.GetCountryCode()),
	}
	if accessToken := strings.TrimSpace(req.GetAccessToken()); accessToken != "" {
		input.Payer = &pb.GoPayWAPaymentWorkflowInput_AccessToken{AccessToken: accessToken}
	} else if accountID := strings.TrimSpace(req.GetAccountId()); accountID != "" {
		input.Payer = &pb.GoPayWAPaymentWorkflowInput_AccountId{AccountId: accountID}
	}
	_, err := s.temporal.ExecuteWorkflow(ctx, s.workflowOptions(workflowIDForAction(actionGoPayWAPayment, jobID)), workflows.GoPayWAPaymentWorkflow, input)
	if err != nil {
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
	_, err := s.temporal.ExecuteWorkflow(ctx, s.workflowOptions(workflowIDForAction(actionGoPayPaymentRebind, jobID)), workflows.GoPayPaymentRebindWorkflow, workflows.GoPayPaymentRebindWorkflowInput{
		JobId:       jobID,
		SourceJobId: sourceJobID,
		AccountId:   strings.TrimSpace(req.GetAccountId()),
		UserId:      strings.TrimSpace(req.GetUserId()),
		Pin:         strings.TrimSpace(req.GetPin()),
		CountryCode: strings.TrimSpace(req.GetCountryCode()),
	})
	if err != nil {
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
	if job.Action != actionGoPayPayment && job.Action != actionGoPayQRISPaymentActivate {
		return &pb.ConfirmManualAddBalanceResponse{Success: false, JobId: jobID, ErrorMessage: "job does not accept add_balance confirmation: " + job.Action}, nil
	}
	if job.LastStep != stepGoPayAppAddBalance {
		return &pb.ConfirmManualAddBalanceResponse{Success: false, JobId: jobID, ErrorMessage: "job is not waiting for add_balance confirmation: " + job.LastStep}, nil
	}
	workflowID, ok := contracts.WorkflowID(job.Action, job.ID)
	if !ok || workflowID == "" {
		return &pb.ConfirmManualAddBalanceResponse{Success: false, JobId: jobID, ErrorMessage: "workflow id not found"}, nil
	}
	if req.GetAddBalance() != nil {
		addBalance := s.mergeDefaultGoPayAddBalance(cloneGoPayAddBalance(req.GetAddBalance()))
		if goPayAddBalanceMethod(addBalance) == "" {
			return &pb.ConfirmManualAddBalanceResponse{Success: false, JobId: jobID, ErrorMessage: "add_balance method is required"}, nil
		}
		if err := s.temporal.SignalWorkflow(ctx, workflowID, "", contracts.GoPayAddBalanceSelectionSignalName, ManualAddBalanceSignal{Kind: "select", AddBalance: addBalance}); err != nil {
			return &pb.ConfirmManualAddBalanceResponse{Success: false, JobId: jobID, ErrorMessage: err.Error()}, nil
		}
		return &pb.ConfirmManualAddBalanceResponse{Success: true, JobId: jobID}, nil
	}
	if err := s.temporal.SignalWorkflow(ctx, workflowID, "", manualAddBalanceSignalName, ManualAddBalanceSignal{Kind: "manual_transfer_confirmed"}); err != nil {
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
	workflowID, ok := contracts.WorkflowID(job.Action, job.ID)
	if !ok || workflowID == "" {
		return &pb.ConfirmManualGoPayPaymentResponse{Success: false, JobId: jobID, ErrorMessage: "workflow id not found"}, nil
	}
	if err := s.temporal.SignalWorkflow(ctx, workflowID, "", manualGoPayPaymentSignalName, ManualGoPayPaymentSignal{Kind: "manual_qris_payment_confirmed"}); err != nil {
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
	_, err := s.temporal.ExecuteWorkflow(ctx, s.workflowOptions(workflowIDForAction(actionRegisterAndActivate, jobID)), workflows.RegisterAndActivateWorkflow, workflows.RegisterAndActivateWorkflowInput{
		JobId: jobID,
		Account: &workflows.AccountSpec{
			AccountId:     accountID,
			Email:         req.GetEmail(),
			Password:      req.GetPassword(),
			EmailStrategy: requestEmailStrategy(req.GetEmailStrategy()),
		},
		OtpOptions:       req.GetOtpOptions(),
		GopayPhone:       req.GetGopayPhone(),
		GopayCountryCode: req.GetGopayCountryCode(),
		GopayPin:         req.GetGopayPin(),
	})
	if err != nil {
		return nil, err
	}
	return &pb.RegisterAndActivateAccountResponse{JobId: jobID, Started: true}, nil
}

func workflowIDForAction(action string, jobID string) string {
	workflowID, ok := contracts.WorkflowID(action, jobID)
	if !ok {
		return jobID
	}
	return workflowID
}

func requestEmailStrategy(strategy pb.AccountEmailStrategy) pb.AccountEmailStrategy {
	if strategy == pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_UNSPECIFIED {
		return pb.AccountEmailStrategy_ACCOUNT_EMAIL_STRATEGY_OUTLOOK_ALIAS
	}
	return strategy
}
