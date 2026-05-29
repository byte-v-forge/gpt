package dashboard

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/byte-v-forge/gpt/pkg/gptplugin"
)

func (s *server) rawN8NWorkflowHandlers() actionHandlerMap {
	out := actionHandlerMap{}
	if s == nil || s.actionRegistry == nil {
		return out
	}
	for _, def := range s.actionRegistry.Actions() {
		if def.Workflow.ActionAPIKind != gptplugin.ActionAPIKindRawN8N {
			continue
		}
		actionID := def.ActionID
		out[actionID] = func(w http.ResponseWriter, r *http.Request) {
			s.handleRawN8NWorkflowStart(actionID, w, r)
		}
	}
	return out
}

func (s *server) handleRawN8NWorkflowStart(actionID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.n8nWorkflowStarter == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n workflow starter is not configured"))
		return
	}
	workflow := s.n8nWorkflow(actionID)
	if workflow == nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("n8n %s workflow is not configured", actionID))
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, rawN8NActionMaxBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("read n8n workflow body: %w", err))
		return
	}
	started, err := s.n8nWorkflowStarter.StartN8NWorkflow(r.Context(), gptplugin.N8NWorkflowStartCall{
		ActionID: actionID,
		RawJSON:  body,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if err := workflow.trigger(r.Context(), started.TriggerPayload); err != nil {
		_ = s.n8nWorkflowStarter.FailN8NWorkflowTrigger(r.Context(), gptplugin.N8NWorkflowTriggerFailure{
			ActionID:     actionID,
			JobID:        started.JobID,
			AccountID:    started.AccountID,
			ErrorMessage: err.Error(),
			Data:         map[string]any{"reason": "n8n_trigger_failed"},
		})
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeRawN8NWorkflowStartedJSON(w, started.Response)
}

func writeRawN8NWorkflowStartedJSON(w http.ResponseWriter, resp any) {
	if started, ok := resp.(startedResponse); ok {
		writeStartedJSON(w, started)
		return
	}
	writeJSON(w, http.StatusAccepted, resp)
}
