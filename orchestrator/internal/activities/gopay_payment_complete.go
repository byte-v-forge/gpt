package activities

import (
	"context"
	"fmt"
	"log"
	"time"

	"orchestrator/pb"
)

func (s *Server) completeGoPayPayment(ctx context.Context, step activityStep, stateJSON, flowID, otp string, useAccountToken bool, pin string, waitForManual bool, data map[string]any) (*pb.GoPayResponse, string, error) {
	stateJSON = normalizeGoPayWorkflowStateJSON(stateJSON)
	completed := false
	defer func() {
		if flowID != "" && !completed {
			cancelCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cancelResp, cancelErr := s.paymentClient.CancelGoPay(cancelCtx, &pb.CancelGoPayRequest{FlowId: flowID})
			data["cleanup"] = cleanupDataFromPayment(cancelResp, cancelErr)
		}
	}()

	result, err := s.paymentClient.CompleteGoPay(ctx, &pb.CompleteGoPayRequest{FlowId: flowID, Otp: otp, Pin: pin})
	data["payment_complete"] = paymentResultData(result)
	data["payment_result_present"] = result != nil
	step.progress("gopay payment complete called", map[string]any{
		"success":                      result != nil && result.GetSuccess(),
		"awaiting_manual_confirmation": result != nil && result.GetAwaitingManualConfirmation(),
	})
	if err != nil {
		return nil, stateJSON, err
	}
	if result == nil {
		return nil, stateJSON, fmt.Errorf("payment complete returned empty response")
	}
	if !result.GetSuccess() {
		return nil, stateJSON, fmt.Errorf("payment complete failed: %s", result.GetErrorMessage())
	}
	if result.GetAwaitingManualConfirmation() {
		data["manual_payment_confirmation"] = map[string]any{
			"required":      true,
			"auto_expected": !waitForManual,
			"confirmed":     false,
		}
		if waitForManual {
			completed = true
			return result, stateJSON, nil
		}
		if !useAccountToken {
			return nil, stateJSON, fmt.Errorf("payment requires manual confirmation; QR autopay did not settle automatically")
		}

		replayResp, nextStateJSON, replayErr := s.replayGoPayPaymentLink(ctx, stateJSON, result, pin)
		stateJSON = nextStateJSON
		data["gopay_payment_replay"] = replayLinkPaymentData(replayResp, replayErr)
		step.progress("gopay payment replayed", map[string]any{
			"success": replayResp != nil && replayResp.GetSuccess(),
		})
		if replayErr != nil {
			return nil, stateJSON, replayErr
		}

		result, err = s.paymentClient.ConfirmGoPayPayment(ctx, &pb.ConfirmGoPayPaymentRequest{FlowId: flowID})
		data["payment_confirm"] = paymentResultData(result)
		data["payment_result_present"] = result != nil
		step.progress("gopay payment confirmed", map[string]any{
			"success": result != nil && result.GetSuccess(),
		})
		if err != nil {
			return nil, stateJSON, err
		}
		if result == nil {
			return nil, stateJSON, fmt.Errorf("payment confirm returned empty response")
		}
		if !result.GetSuccess() {
			return nil, stateJSON, fmt.Errorf("payment confirm failed: %s", result.GetErrorMessage())
		}
		data["manual_payment_confirmation"] = map[string]any{
			"required":      true,
			"auto_expected": true,
			"confirmed":     true,
		}
	}
	completed = true

	if useAccountToken {
		nextStateJSON, unlinkErr := s.unlinkGoPayAccountToken(ctx, stateJSON)
		stateJSON = nextStateJSON
		if unlinkErr != nil {
			log.Printf("[gopay-app] Unlink after payment failed: %v", unlinkErr)
			data["gopay_unlink"] = cleanupData(false, unlinkErr.Error(), unlinkErr)
		} else {
			data["gopay_unlink"] = cleanupData(true, "", nil)
		}
	}
	return result, stateJSON, nil
}

func (s *Server) completeGoPayPaymentStep(ctx context.Context, jobID, accountID string, data map[string]any, err error) error {
	if err != nil && isFreeTrialIneligibleError(err) {
		if updateErr := s.updateAccount(ctx, &pb.Account{
			AccountId:         accountID,
			PlusTrialEligible: boolPtr(false),
		}); updateErr != nil {
			err = fmt.Errorf("%w; additionally failed to mark plus trial ineligible: %v", err, updateErr)
		}
	}
	return s.completeActivityStep(ctx, jobID, stepGoPayPayment, false, true, data, err)
}
