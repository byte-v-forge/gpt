package api

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/byte-v-forge/common-lib/protojsonx"
	"github.com/byte-v-forge/gpt/pkg/gptplugin"
	"google.golang.org/protobuf/proto"

	"orchestrator/pb"
)

func (s *Server) StartN8NWorkflow(ctx context.Context, call gptplugin.N8NWorkflowStartCall) (gptplugin.N8NWorkflowStartResult, error) {
	actionID := strings.ToUpper(strings.TrimSpace(call.ActionID))
	switch actionID {
	case actionGoPayApp:
		var req pb.GoPayAppRequest
		if err := decodeN8NWorkflowRequest(call.RawJSON, &req); err != nil {
			return gptplugin.N8NWorkflowStartResult{}, err
		}
		resp, accountID, err := s.StartN8NGoPayApp(ctx, &req)
		return newN8NWorkflowStartResult(resp, resp.GetJobId(), accountID), err
	case actionGoPayPayment:
		var req pb.GoPayPaymentRequest
		if err := decodeN8NWorkflowRequest(call.RawJSON, &req); err != nil {
			return gptplugin.N8NWorkflowStartResult{}, err
		}
		resp, accountID, err := s.StartN8NGoPayPayment(ctx, &req)
		return newN8NWorkflowStartResult(resp, resp.GetJobId(), accountID), err
	case actionGoPayQRISPaymentActivate:
		var req pb.GoPayQRISPaymentActivateRequest
		if err := decodeN8NWorkflowRequest(call.RawJSON, &req); err != nil {
			return gptplugin.N8NWorkflowStartResult{}, err
		}
		resp, accountID, err := s.StartN8NGoPayQRISPaymentActivate(ctx, &req)
		return newN8NWorkflowStartResult(resp, resp.GetJobId(), accountID), err
	case actionGoPayWAPayment:
		var req pb.GoPayWAPaymentRequest
		if err := decodeN8NWorkflowRequest(call.RawJSON, &req); err != nil {
			return gptplugin.N8NWorkflowStartResult{}, err
		}
		resp, accountID, err := s.StartN8NGoPayWAPayment(ctx, &req)
		return newN8NWorkflowStartResult(resp, resp.GetJobId(), accountID), err
	case actionGoPayPaymentRebind:
		var req pb.GoPayPaymentRebindRequest
		if err := decodeN8NWorkflowRequest(call.RawJSON, &req); err != nil {
			return gptplugin.N8NWorkflowStartResult{}, err
		}
		resp, accountID, err := s.StartN8NGoPayPaymentRebind(ctx, &req)
		return newN8NWorkflowStartResult(resp, resp.GetJobId(), accountID), err
	default:
		return gptplugin.N8NWorkflowStartResult{}, fmt.Errorf("unsupported raw n8n workflow: %s", actionID)
	}
}

func (s *Server) FailN8NWorkflowTrigger(ctx context.Context, failure gptplugin.N8NWorkflowTriggerFailure) error {
	actionID := strings.ToUpper(strings.TrimSpace(failure.ActionID))
	switch actionID {
	case actionGoPayApp, actionGoPayPayment, actionGoPayQRISPaymentActivate, actionGoPayWAPayment, actionGoPayPaymentRebind:
		data := failure.Data
		if data == nil {
			data = map[string]any{}
		}
		if strings.TrimSpace(failure.AccountID) != "" {
			data["account_id"] = strings.TrimSpace(failure.AccountID)
		}
		_, err := s.FailN8NGoPay(ctx, actionID, failure.JobID, failure.N8NExecutionID, failure.ErrorMessage, data)
		return err
	default:
		return fmt.Errorf("unsupported raw n8n workflow failure: %s", actionID)
	}
}

func decodeN8NWorkflowRequest(raw []byte, dst proto.Message) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte("{}")
	}
	if err := protojsonx.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("decode n8n workflow request: %w", err)
	}
	return nil
}

func newN8NWorkflowStartResult(resp any, jobID string, accountID string) gptplugin.N8NWorkflowStartResult {
	payload := map[string]string{"job_id": strings.TrimSpace(jobID)}
	if accountID = strings.TrimSpace(accountID); accountID != "" {
		payload["account_id"] = accountID
	}
	return gptplugin.N8NWorkflowStartResult{
		Response:       resp,
		JobID:          strings.TrimSpace(jobID),
		AccountID:      accountID,
		TriggerPayload: payload,
	}
}
