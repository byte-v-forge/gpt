package paymentsvc

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/byte-v-forge/gpt/gopay/pb"
	"github.com/google/uuid"
)

type Server struct {
	pb.UnimplementedPaymentServiceServer
	cfg   Config
	flows *flowStore
}

func NewServer(cfg Config) *Server {
	return &Server{cfg: cfg, flows: &flowStore{items: map[string]*pendingFlow{}}}
}

type pendingFlow struct {
	charger         *charger
	state           map[string]any
	useAccountToken bool
}

func (f *pendingFlow) close() {
	if f != nil && f.charger != nil {
		f.charger.close()
	}
}

type flowStore struct {
	mu    sync.Mutex
	items map[string]*pendingFlow
}

func (s *flowStore) put(flow *pendingFlow) string {
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[id] = flow
	return id
}

func (s *flowStore) get(id string) *pendingFlow {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.items[strings.TrimSpace(id)]
}

func (s *flowStore) pop(id string) *pendingFlow {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	flow := s.items[id]
	delete(s.items, id)
	return flow
}

func (s *flowStore) close() {
	s.mu.Lock()
	flows := make([]*pendingFlow, 0, len(s.items))
	for _, flow := range s.items {
		flows = append(flows, flow)
	}
	s.items = map[string]*pendingFlow{}
	s.mu.Unlock()
	for _, flow := range flows {
		flow.close()
	}
}

type credential struct {
	sessionToken string
	accessToken  string
}

func requestCredential(value *pb.ChatGPTCredential) credential {
	if value == nil {
		return credential{}
	}
	return credential{
		sessionToken: strings.TrimSpace(value.GetSessionToken()),
		accessToken:  strings.TrimSpace(value.GetAccessToken()),
	}
}

func (c credential) empty() bool {
	return c.sessionToken == "" && c.accessToken == ""
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

func truncateError(err error) string {
	text := errorText(err)
	if len(text) > 500 {
		return text[:500]
	}
	return text
}

func (s *Server) ProbeTier(ctx context.Context, req *pb.ProbeTierPaymentRequest) (*pb.ProbeTierPaymentResponse, error) {
	cred := requestCredential(req.GetCredential())
	if cred.empty() {
		return &pb.ProbeTierPaymentResponse{Success: false, ErrorMessage: "credential is required"}, nil
	}
	result, err := s.probeTier(ctx, cred)
	if err != nil {
		return &pb.ProbeTierPaymentResponse{Success: false, ErrorMessage: truncateError(err)}, nil
	}
	return &pb.ProbeTierPaymentResponse{
		Success:      result.ErrorMessage == "",
		ErrorMessage: result.ErrorMessage,
		Checked:      result.Checked,
		Tier:         firstNonEmpty(result.Tier, result.PlanType),
		PlusActive:   result.PlusActive,
		Source:       firstNonEmpty(result.Source, "auth_session"),
	}, nil
}

func (s *Server) ProbePlusTrial(ctx context.Context, req *pb.ProbePlusTrialPaymentRequest) (*pb.ProbePlusTrialPaymentResponse, error) {
	cred := requestCredential(req.GetCredential())
	if cred.empty() {
		return &pb.ProbePlusTrialPaymentResponse{Success: false, ErrorMessage: "session_token or access_token is required"}, nil
	}
	result, err := s.probePlusTrial(ctx, cred)
	if err != nil {
		return &pb.ProbePlusTrialPaymentResponse{Success: false, ErrorMessage: truncateError(err)}, nil
	}
	return &pb.ProbePlusTrialPaymentResponse{
		Success:           result.ErrorMessage == "",
		ErrorMessage:      result.ErrorMessage,
		Checked:           result.Checked,
		PlusTrialEligible: result.PlusTrialEligible,
		Amount:            result.Amount,
		Currency:          result.Currency,
		Source:            result.Source,
		CheckoutUrl:       result.CheckoutURL,
		CheckoutSessionId: result.CheckoutSessionID,
		PlusActive:        result.PlusActive,
		PlanType:          result.PlanType,
	}, nil
}

func (s *Server) CreateCheckoutLink(ctx context.Context, req *pb.CreateCheckoutLinkRequest) (*pb.CreateCheckoutLinkResponse, error) {
	cred := requestCredential(req.GetCredential())
	if cred.empty() {
		return &pb.CreateCheckoutLinkResponse{Success: false, ErrorMessage: "session_token or access_token is required"}, nil
	}
	ch, err := s.newCharger(ctx, cred, "", "", "", defaultTokenization)
	if err != nil {
		return &pb.CreateCheckoutLinkResponse{Success: false, ErrorMessage: truncateError(err)}, nil
	}
	defer ch.close()
	csID, err := ch.createCheckout(ctx)
	if err != nil {
		return &pb.CreateCheckoutLinkResponse{Success: false, ErrorMessage: truncateError(err)}, nil
	}
	return &pb.CreateCheckoutLinkResponse{Success: true, CheckoutUrl: ch.checkoutURL, CheckoutSessionId: csID}, nil
}

func (s *Server) PrepareGoPay(ctx context.Context, req *pb.PrepareGoPayRequest) (*pb.PrepareGoPayResponse, error) {
	cred := requestCredential(req.GetCredential())
	if cred.empty() {
		return &pb.PrepareGoPayResponse{Success: false, ErrorMessage: "session_token or access_token is required"}, nil
	}
	ch, err := s.newCharger(ctx, cred, req.GetGopayPhone(), req.GetGopayCountryCode(), "", req.GetTokenization())
	if err != nil {
		return &pb.PrepareGoPayResponse{Success: false, ErrorMessage: truncateError(err)}, nil
	}
	state, err := ch.prepareUntilLinking(ctx, req.GetCheckoutSessionId(), req.GetCheckoutUrl())
	if err != nil {
		ch.close()
		return &pb.PrepareGoPayResponse{Success: false, ErrorMessage: truncateError(err)}, nil
	}
	flowID := s.flows.put(&pendingFlow{charger: ch, state: state})
	return &pb.PrepareGoPayResponse{
		Success:           true,
		FlowId:            flowID,
		SnapToken:         stringAt(state, "snap_token"),
		CheckoutUrl:       stringAt(state, "checkout_url"),
		CheckoutSessionId: stringAt(state, "cs_id"),
		CheckoutAttempt:   int32(intAt(state, "checkout_attempt")),
		Stage:             stringAt(state, "state"),
	}, nil
}

func (s *Server) PrepareGoPayCheckout(ctx context.Context, req *pb.PrepareGoPayCheckoutRequest) (*pb.PrepareGoPayResponse, error) {
	cred := requestCredential(req.GetCredential())
	if cred.empty() {
		return &pb.PrepareGoPayResponse{Success: false, ErrorMessage: "session_token or access_token is required", Stage: "checkout"}, nil
	}
	ch, err := s.newCharger(ctx, cred, req.GetGopayPhone(), req.GetGopayCountryCode(), "", req.GetTokenization())
	if err != nil {
		return &pb.PrepareGoPayResponse{Success: false, ErrorMessage: truncateError(err), Stage: "checkout"}, nil
	}
	csID, state, err := ch.prepareCheckout(ctx, req.GetCheckoutSessionId(), req.GetCheckoutUrl(), 1)
	if err != nil {
		ch.close()
		return &pb.PrepareGoPayResponse{Success: false, ErrorMessage: truncateError(err), Stage: "checkout"}, nil
	}
	flowID := s.flows.put(&pendingFlow{charger: ch, state: state})
	return &pb.PrepareGoPayResponse{
		Success:           true,
		FlowId:            flowID,
		CheckoutUrl:       stringAt(state, "checkout_url"),
		CheckoutSessionId: csID,
		CheckoutAttempt:   int32(intAt(state, "checkout_attempt")),
		Stage:             "checkout",
	}, nil
}

func (s *Server) RefreshPrepareGoPayCheckout(ctx context.Context, req *pb.RefreshPrepareGoPayCheckoutRequest) (*pb.PrepareGoPayResponse, error) {
	flowID := strings.TrimSpace(req.GetFlowId())
	if flowID == "" {
		return &pb.PrepareGoPayResponse{Success: false, ErrorMessage: "flow_id is required", Stage: "checkout_refresh"}, nil
	}
	flow := s.flows.get(flowID)
	if flow == nil {
		return &pb.PrepareGoPayResponse{Success: false, ErrorMessage: "prepared payment flow not found", Stage: "checkout_refresh"}, nil
	}
	nextAttempt := int(intAt(flow.state, "checkout_attempt")) + 1
	if nextAttempt <= 1 {
		nextAttempt = 2
	}
	csID, state, err := flow.charger.prepareCheckout(ctx, "", "", nextAttempt)
	if err != nil {
		if failed := s.flows.pop(flowID); failed != nil {
			failed.close()
		}
		return &pb.PrepareGoPayResponse{Success: false, ErrorMessage: truncateError(err), FlowId: flowID, Stage: "checkout_refresh"}, nil
	}
	flow.state = state
	return &pb.PrepareGoPayResponse{
		Success:           true,
		FlowId:            flowID,
		CheckoutUrl:       stringAt(state, "checkout_url"),
		CheckoutSessionId: csID,
		CheckoutAttempt:   int32(intAt(state, "checkout_attempt")),
		Stage:             "checkout_refresh",
	}, nil
}

func (s *Server) PrepareGoPayLink(ctx context.Context, req *pb.PrepareGoPayLinkRequest) (*pb.PrepareGoPayResponse, error) {
	flowID := strings.TrimSpace(req.GetFlowId())
	if flowID == "" {
		return &pb.PrepareGoPayResponse{Success: false, ErrorMessage: "flow_id is required", Stage: "link"}, nil
	}
	flow := s.flows.get(flowID)
	if flow == nil {
		return &pb.PrepareGoPayResponse{Success: false, ErrorMessage: "prepared payment flow not found", FlowId: flowID, Stage: "link"}, nil
	}
	csID := stringAt(flow.state, "cs_id")
	if csID == "" {
		return &pb.PrepareGoPayResponse{Success: false, ErrorMessage: "prepared payment flow is missing checkout_session_id", FlowId: flowID, Stage: "link"}, nil
	}
	state, err := flow.charger.prepareCheckoutSessionUntilLinking(ctx, csID)
	if err != nil {
		if isChatGPTApproveBlocked(err) {
			return &pb.PrepareGoPayResponse{
				Success:                false,
				ErrorMessage:           truncateError(err),
				FlowId:                 flowID,
				CheckoutUrl:            stringAt(flow.state, "checkout_url"),
				CheckoutSessionId:      csID,
				RetryableFreshCheckout: true,
				CheckoutAttempt:        int32(intAt(flow.state, "checkout_attempt")),
				Stage:                  "link",
			}, nil
		}
		if failed := s.flows.pop(flowID); failed != nil {
			failed.close()
		}
		return &pb.PrepareGoPayResponse{
			Success:           false,
			ErrorMessage:      truncateError(err),
			FlowId:            flowID,
			CheckoutUrl:       stringAt(flow.state, "checkout_url"),
			CheckoutSessionId: csID,
			CheckoutAttempt:   int32(intAt(flow.state, "checkout_attempt")),
			Stage:             "link",
		}, nil
	}
	state["checkout_attempt"] = intAt(flow.state, "checkout_attempt")
	flow.state = state
	return &pb.PrepareGoPayResponse{
		Success:           true,
		FlowId:            flowID,
		SnapToken:         stringAt(state, "snap_token"),
		CheckoutUrl:       stringAt(state, "checkout_url"),
		CheckoutSessionId: stringAt(state, "cs_id"),
		CheckoutAttempt:   int32(intAt(state, "checkout_attempt")),
		Stage:             "link",
	}, nil
}

func (s *Server) StartGoPay(ctx context.Context, req *pb.StartGoPayRequest) (*pb.StartGoPayResponse, error) {
	cred := requestCredential(req.GetCredential())
	if req.GetUseAccountToken() && cred.accessToken == "" {
		return &pb.StartGoPayResponse{Success: false, ErrorMessage: "access_token is required when use_account_token=true"}, nil
	}
	if cred.empty() {
		return &pb.StartGoPayResponse{Success: false, ErrorMessage: "session_token or access_token is required"}, nil
	}
	ch, err := s.newCharger(ctx, cred, req.GetGopayPhone(), req.GetGopayCountryCode(), "", req.GetTokenization())
	if err != nil {
		return &pb.StartGoPayResponse{Success: false, ErrorMessage: truncateError(err)}, nil
	}
	state, err := ch.startUntilOTP(ctx, req.GetCheckoutSessionId(), req.GetCheckoutUrl(), req.GetOtpChannel())
	if err != nil {
		ch.close()
		return &pb.StartGoPayResponse{Success: false, ErrorMessage: truncateError(err)}, nil
	}
	flowID := s.flows.put(&pendingFlow{charger: ch, state: state, useAccountToken: req.GetUseAccountToken()})
	return startResponse(flowID, state), nil
}

func (s *Server) StartPreparedGoPay(ctx context.Context, req *pb.StartPreparedGoPayRequest) (*pb.StartGoPayResponse, error) {
	flowID := strings.TrimSpace(req.GetFlowId())
	if flowID == "" {
		return &pb.StartGoPayResponse{Success: false, ErrorMessage: "flow_id is required"}, nil
	}
	flow := s.flows.get(flowID)
	if flow == nil {
		return &pb.StartGoPayResponse{Success: false, ErrorMessage: "prepared payment flow not found"}, nil
	}
	if phone := strings.TrimSpace(req.GetGopayPhone()); phone != "" {
		flow.charger.phone = normalizeDigits(phone)
	}
	if country := normalizeCountryCode(req.GetGopayCountryCode()); country != "" {
		flow.charger.countryCode = country
	}
	var state map[string]any
	var err error
	if flow.charger.requiresManualConfirmation() {
		state, err = flow.charger.startPreparedQRISToPaymentCharge(ctx, flow.state)
	} else {
		state, err = flow.charger.startPreparedLinkingUntilOTP(ctx, flow.state, req.GetOtpChannel())
	}
	if err != nil {
		if failed := s.flows.pop(flowID); failed != nil {
			failed.close()
		}
		return &pb.StartGoPayResponse{Success: false, ErrorMessage: truncateError(err)}, nil
	}
	flow.state = state
	return startResponse(flowID, state), nil
}

func (s *Server) ResendGoPayOTP(ctx context.Context, req *pb.ResendGoPayOTPRequest) (*pb.ResendGoPayOTPResponse, error) {
	flowID := strings.TrimSpace(req.GetFlowId())
	if flowID == "" {
		return &pb.ResendGoPayOTPResponse{Success: false, ErrorMessage: "flow_id is required"}, nil
	}
	flow := s.flows.get(flowID)
	if flow == nil {
		return &pb.ResendGoPayOTPResponse{Success: false, ErrorMessage: "payment flow not found"}, nil
	}
	state, err := flow.charger.resendLinkingOTP(ctx, flow.state)
	if err != nil {
		return &pb.ResendGoPayOTPResponse{Success: false, FlowId: flowID, ErrorMessage: truncateError(err)}, nil
	}
	flow.state = state
	return &pb.ResendGoPayOTPResponse{Success: true, FlowId: flowID, IssuedAfterUnix: int64(intAt(state, "issued_after_unix"))}, nil
}

func (s *Server) CompleteGoPay(ctx context.Context, req *pb.CompleteGoPayRequest) (*pb.GoPayResponse, error) {
	flowID := strings.TrimSpace(req.GetFlowId())
	if flowID == "" {
		return &pb.GoPayResponse{Success: false, ErrorMessage: "flow_id is required"}, nil
	}
	flow := s.flows.get(flowID)
	if flow == nil {
		return &pb.GoPayResponse{Success: false, ErrorMessage: "payment flow not found"}, nil
	}
	if boolAt(flow.state, "otp_required") && strings.TrimSpace(req.GetOtp()) == "" {
		return &pb.GoPayResponse{Success: false, ErrorMessage: "otp is required"}, nil
	}
	if pin := strings.TrimSpace(req.GetPin()); pin != "" {
		flow.charger.pin = pin
	}
	closeFlow := true
	var result map[string]any
	var err error
	if flow.charger.requiresManualConfirmation() {
		if boolAt(flow.state, "otp_required") {
			result, err = flow.charger.completeAfterOTPUntilManualConfirmation(ctx, flow.state, req.GetOtp())
			flow.state = result
		} else {
			result = flow.state
		}
		closeFlow = false
	} else if boolAt(flow.state, "otp_required") {
		result, err = flow.charger.completeAfterOTP(ctx, flow.state, req.GetOtp())
	} else {
		result, err = flow.charger.completeAfterManualConfirmation(ctx, flow.state)
	}
	if err != nil {
		if closeFlow {
			if failed := s.flows.pop(flowID); failed != nil {
				failed.close()
			}
		}
		return &pb.GoPayResponse{Success: false, ErrorMessage: truncateError(err)}, nil
	}
	if closeFlow {
		if done := s.flows.pop(flowID); done != nil {
			done.close()
		}
	}
	if !closeFlow {
		flow.state = result
	}
	return paymentResponse(result, !closeFlow), nil
}

func (s *Server) ConfirmGoPayPayment(ctx context.Context, req *pb.ConfirmGoPayPaymentRequest) (*pb.GoPayResponse, error) {
	flowID := strings.TrimSpace(req.GetFlowId())
	if flowID == "" {
		return &pb.GoPayResponse{Success: false, ErrorMessage: "flow_id is required"}, nil
	}
	flow := s.flows.get(flowID)
	if flow == nil {
		return &pb.GoPayResponse{Success: false, ErrorMessage: "payment flow not found"}, nil
	}
	result, err := flow.charger.completeAfterManualConfirmation(ctx, flow.state)
	if err != nil {
		return &pb.GoPayResponse{Success: false, ErrorMessage: truncateError(err)}, nil
	}
	if done := s.flows.pop(flowID); done != nil {
		done.close()
	}
	return paymentResponse(result, false), nil
}

func (s *Server) CancelGoPay(_ context.Context, req *pb.CancelGoPayRequest) (*pb.CancelGoPayResponse, error) {
	if flowID := strings.TrimSpace(req.GetFlowId()); flowID != "" {
		if flow := s.flows.pop(flowID); flow != nil {
			flow.close()
		}
	}
	return &pb.CancelGoPayResponse{Success: true}, nil
}

func startResponse(flowID string, state map[string]any) *pb.StartGoPayResponse {
	return &pb.StartGoPayResponse{
		Success:           true,
		FlowId:            flowID,
		SnapToken:         stringAt(state, "snap_token"),
		IssuedAfterUnix:   intAt(state, "issued_after_unix"),
		CheckoutUrl:       stringAt(state, "checkout_url"),
		CheckoutSessionId: firstNonEmpty(stringAt(state, "cs_id"), stringAt(state, "checkout_session_id")),
		OtpRequired:       boolAt(state, "otp_required"),
	}
}

func paymentResponse(result map[string]any, awaitingManual bool) *pb.GoPayResponse {
	state := stringAt(result, "state")
	success := awaitingManual || state == "succeeded"
	message := ""
	if !success {
		message = fmt.Sprintf("payment state=%s", firstNonEmpty(state, "unknown"))
	}
	return &pb.GoPayResponse{
		Success:                    success,
		ErrorMessage:               message,
		ChargeRef:                  stringAt(result, "charge_ref"),
		SnapToken:                  stringAt(result, "snap_token"),
		AwaitingManualConfirmation: awaitingManual,
		DeeplinkUrl:                stringAt(result, "deeplink_url"),
		QrCodeUrl:                  stringAt(result, "qr_code_url"),
		FinishRedirectUrl:          stringAt(result, "finish_redirect_url"),
		Finish_200RedirectUrl:      stringAt(result, "finish_200_redirect_url"),
		QrString:                   stringAt(result, "qr_string"),
	}
}
