package activities

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	pb "orchestrator/pb"
)

func (s *Server) gopayAppStepResponseBodyLimit() int32 {
	if s.gopayAppStepBodyLimit <= 0 {
		return 6000
	}
	return s.gopayAppStepBodyLimit
}

func (s *Server) gopayAppLinkPaymentWaitTimeout() time.Duration {
	if s.gopayAppLinkPaymentTimeout <= 0 {
		return 180 * time.Second
	}
	return s.gopayAppLinkPaymentTimeout
}

func (s *Server) gopayAppUnlinkWaitTimeout() time.Duration {
	if s.gopayAppUnlinkTimeout <= 0 {
		return 15 * time.Second
	}
	return s.gopayAppUnlinkTimeout
}

func (s *Server) readyGoPayAccountToken(ctx context.Context, stateJSON string) (string, string, string, error) {
	if s.gopayClient == nil {
		return "", "", normalizeGoPayWorkflowStateJSON(stateJSON), fmt.Errorf("gopay-app client not configured")
	}
	resp, err := s.gopayClient.GetReadyAccountToken(ctx, &pb.GetReadyAccountTokenRequest{StateJson: stateJSON})
	nextStateJSON := goPayWorkflowStateAfter(stateJSON, responseStateJSON(resp))
	if err != nil {
		return "", "", nextStateJSON, fmt.Errorf("GetReadyAccountToken: %w", err)
	}
	if resp == nil {
		return "", "", nextStateJSON, fmt.Errorf("GetReadyAccountToken returned empty response")
	}
	if !resp.GetSuccess() {
		return "", "", nextStateJSON, fmt.Errorf("GetReadyAccountToken: %s", resp.GetErrorMessage())
	}
	token := strings.TrimSpace(resp.GetAccountToken())
	if token == "" {
		return "", "", nextStateJSON, fmt.Errorf("GetReadyAccountToken returned empty account token")
	}
	return token, strings.TrimSpace(resp.GetPhone()), nextStateJSON, nil
}

func paymentLinkFromGoPayResponse(resp *pb.GoPayResponse) (string, error) {
	if resp == nil {
		return "", fmt.Errorf("payment response is empty")
	}
	for _, value := range []string{resp.GetDeeplinkUrl(), resp.GetChargeRef()} {
		if link := strings.TrimSpace(value); link != "" {
			return link, nil
		}
	}
	return "", fmt.Errorf("midtrans payment link is missing")
}

func (s *Server) replayGoPayPaymentLink(ctx context.Context, stateJSON string, paymentResp *pb.GoPayResponse) (*pb.ReplayLinkPaymentResponse, string, error) {
	if s.gopayClient == nil {
		return nil, normalizeGoPayWorkflowStateJSON(stateJSON), fmt.Errorf("gopay-app client not configured")
	}
	paymentLink, err := paymentLinkFromGoPayResponse(paymentResp)
	if err != nil {
		return nil, normalizeGoPayWorkflowStateJSON(stateJSON), err
	}
	pin := configuredGoPayPIN()
	if pin == "" {
		return nil, normalizeGoPayWorkflowStateJSON(stateJSON), fmt.Errorf("GOPAY_PIN is required")
	}
	timeout := s.gopayAppLinkPaymentWaitTimeout()
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resp, err := s.gopayClient.ReplayLinkPayment(reqCtx, &pb.ReplayLinkPaymentRequest{
		StateJson:   stateJSON,
		PaymentLink: paymentLink,
		Pin:         pin,
		BodyLimit:   s.gopayAppStepResponseBodyLimit(),
	})
	nextStateJSON := goPayWorkflowStateAfter(stateJSON, responseStateJSON(resp))
	if err != nil {
		return resp, nextStateJSON, fmt.Errorf("ReplayLinkPayment: %w", err)
	}
	if resp == nil {
		return nil, nextStateJSON, fmt.Errorf("ReplayLinkPayment returned empty response")
	}
	if !resp.GetSuccess() {
		for _, step := range resp.GetSteps() {
			log.Printf("[gopay-app] ReplayLinkPayment step label=%s status=%d error=%s response=%s",
				step.GetLabel(),
				step.GetStatusCode(),
				step.GetErrorMessage(),
				strings.TrimSpace(step.GetResponseText()),
			)
		}
		return resp, nextStateJSON, fmt.Errorf("ReplayLinkPayment: %s", resp.GetErrorMessage())
	}
	return resp, nextStateJSON, nil
}

func (s *Server) unlinkGoPayAccountToken(ctx context.Context, stateJSON string) (string, error) {
	if s.gopayClient == nil {
		return normalizeGoPayWorkflowStateJSON(stateJSON), fmt.Errorf("gopay-app client not configured")
	}
	timeout := s.gopayAppUnlinkWaitTimeout()
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resp, err := s.gopayClient.Unlink(reqCtx, &pb.UnlinkRequest{StateJson: stateJSON})
	nextStateJSON := goPayWorkflowStateAfter(stateJSON, responseStateJSON(resp))
	if err != nil {
		return nextStateJSON, fmt.Errorf("Unlink: %w", err)
	}
	if resp == nil {
		return nextStateJSON, fmt.Errorf("Unlink returned empty response")
	}
	if !resp.GetSuccess() {
		return nextStateJSON, fmt.Errorf("Unlink: %s", resp.GetErrorMessage())
	}
	log.Printf("[gopay-app] Unlinked token-linked apps count=%d", resp.GetUnlinkedCount())
	return nextStateJSON, nil
}

func goPayStatusTokenReady(resp *pb.StatusResponse) bool {
	return resp != nil && resp.GetStage() == "ready" && resp.GetTokenPresent()
}

func (s *Server) validateGoPayAccountToken(ctx context.Context) (*pb.CheckTokenValidResponse, error) {
	stateJSON, err := s.loadGoPayAppState(ctx)
	if err != nil {
		return nil, err
	}
	resp, _, err := s.validateGoPayAccountTokenForState(ctx, stateJSON)
	if err == nil && resp != nil {
		err = s.saveGoPayAppState(ctx, resp.GetStateJson())
	}
	return resp, err
}

func (s *Server) validateGoPayAccountTokenForState(ctx context.Context, stateJSON string) (*pb.CheckTokenValidResponse, string, error) {
	if s.gopayClient == nil {
		return nil, normalizeGoPayWorkflowStateJSON(stateJSON), fmt.Errorf("gopay app client not configured")
	}
	resp, err := s.gopayClient.CheckTokenValid(ctx, &pb.CheckTokenValidRequest{StateJson: stateJSON})
	nextStateJSON := goPayWorkflowStateAfter(stateJSON, responseStateJSON(resp))
	if err != nil {
		return nil, nextStateJSON, err
	}
	if resp == nil {
		return nil, nextStateJSON, fmt.Errorf("empty response")
	}
	return resp, nextStateJSON, nil
}

func (s *Server) waitForGoPayMinBalance(ctx context.Context, step activityStep, stateJSON string) (string, error) {
	if s.gopayClient == nil {
		return normalizeGoPayWorkflowStateJSON(stateJSON), fmt.Errorf("gopay app client not configured")
	}
	deadline := time.NewTimer(gopayBalanceWaitTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(gopayBalancePollInterval)
	defer ticker.Stop()

	var lastAmount int64
	lastCurrency := "IDR"
	var lastErr string
	for {
		step.progress("checking gopay balance", map[string]any{
			"last_amount":   lastAmount,
			"last_currency": lastCurrency,
			"last_error":    lastErr,
		})
		var resp *pb.CheckTokenValidResponse
		var err error
		resp, stateJSON, err = s.validateGoPayAccountTokenForState(ctx, stateJSON)
		if resp != nil {
			stateJSON = goPayWorkflowStateAfter(stateJSON, resp.GetStateJson())
		}
		if err != nil {
			lastErr = err.Error()
		} else if resp == nil {
			lastErr = "empty response"
		} else {
			lastAmount = resp.GetBalanceAmount()
			if strings.TrimSpace(resp.GetBalanceCurrency()) != "" {
				lastCurrency = resp.GetBalanceCurrency()
			}
			if !resp.GetTokenValid() {
				message := strings.TrimSpace(resp.GetErrorMessage())
				if message == "" {
					message = "token invalid"
				}
				return stateJSON, fmt.Errorf("%s", message)
			}
			if resp.GetSuccess() && resp.GetHasMinBalance() {
				return stateJSON, nil
			}
			lastErr = strings.TrimSpace(resp.GetErrorMessage())
		}

		select {
		case <-ticker.C:
			continue
		case <-deadline.C:
			if lastErr != "" {
				return stateJSON, fmt.Errorf("gopay balance not ready after %s: %d %s; last_error=%s", gopayBalanceWaitTimeout, lastAmount, lastCurrency, lastErr)
			}
			return stateJSON, fmt.Errorf("gopay balance not ready after %s: %d %s", gopayBalanceWaitTimeout, lastAmount, lastCurrency)
		case <-ctx.Done():
			return stateJSON, ctx.Err()
		}
	}
}
