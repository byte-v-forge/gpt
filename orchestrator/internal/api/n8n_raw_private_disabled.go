//go:build !private_plugins

package api

import (
	"context"
	"fmt"

	"github.com/byte-v-forge/gpt/pkg/gptplugin"

	"orchestrator/internal/contracts"
)

func (s *Server) InvokeN8NAction(ctx context.Context, call gptplugin.N8NActionCall) (any, error) {
	actionID := contracts.NormalizeActionID(call.ActionID)
	switch actionID {
	case actionGoPayPayment, actionGoPayQRISPaymentActivate, actionGoPayWAPayment, actionGoPayPaymentRebind:
		return s.invokeN8NGoPayHostAction(ctx, actionID, call.SubPath, call.RawJSON)
	default:
		if actionID == "" {
			actionID = "unknown"
		}
		return nil, fmt.Errorf("private raw n8n action is not available in core runtime: %s", actionID)
	}
}

func (s *Server) StartN8NWorkflow(ctx context.Context, call gptplugin.N8NWorkflowStartCall) (gptplugin.N8NWorkflowStartResult, error) {
	actionID := contracts.NormalizeActionID(call.ActionID)
	switch actionID {
	case actionGoPayPayment, actionGoPayQRISPaymentActivate:
		return s.startN8NGoPayWorkflow(ctx, actionID, call.RawJSON)
	default:
		if actionID == "" {
			actionID = "unknown"
		}
		return gptplugin.N8NWorkflowStartResult{}, fmt.Errorf("private raw n8n workflow is not available in core runtime: %s", actionID)
	}
}

func (s *Server) FailN8NWorkflowTrigger(ctx context.Context, failure gptplugin.N8NWorkflowTriggerFailure) error {
	actionID := contracts.NormalizeActionID(failure.ActionID)
	switch actionID {
	case actionGoPayPayment, actionGoPayQRISPaymentActivate:
		data := failure.Data
		if data == nil {
			data = map[string]any{}
		}
		if failure.AccountID != "" {
			data["account_id"] = failure.AccountID
		}
		_, err := s.failN8NGoPayHostAction(ctx, actionID, failure.JobID, failure.N8NExecutionID, failure.ErrorMessage, data)
		return err
	default:
		if actionID == "" {
			actionID = "unknown"
		}
		return fmt.Errorf("private raw n8n workflow failure handler is not available in core runtime: %s", actionID)
	}
}
