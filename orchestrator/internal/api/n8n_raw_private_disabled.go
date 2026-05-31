//go:build !private_plugins

package api

import (
	"context"
	"fmt"

	"github.com/byte-v-forge/gpt/pkg/gptplugin"

	"orchestrator/internal/contracts"
)

func (s *Server) InvokeN8NAction(_ context.Context, call gptplugin.N8NActionCall) (any, error) {
	actionID := contracts.NormalizeActionID(call.ActionID)
	if actionID == "" {
		actionID = "unknown"
	}
	return nil, fmt.Errorf("private raw n8n action is not available in core runtime: %s", actionID)
}

func (s *Server) StartN8NWorkflow(_ context.Context, call gptplugin.N8NWorkflowStartCall) (gptplugin.N8NWorkflowStartResult, error) {
	actionID := contracts.NormalizeActionID(call.ActionID)
	if actionID == "" {
		actionID = "unknown"
	}
	return gptplugin.N8NWorkflowStartResult{}, fmt.Errorf("private raw n8n workflow is not available in core runtime: %s", actionID)
}

func (s *Server) FailN8NWorkflowTrigger(_ context.Context, failure gptplugin.N8NWorkflowTriggerFailure) error {
	actionID := contracts.NormalizeActionID(failure.ActionID)
	if actionID == "" {
		actionID = "unknown"
	}
	return fmt.Errorf("private raw n8n workflow failure handler is not available in core runtime: %s", actionID)
}
