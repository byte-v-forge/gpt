package dashboard

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/byte-v-forge/gpt/pkg/gptplugin"
)

const rawN8NActionMaxBodyBytes = 1 << 20

func (s *server) rawN8NActionHandlers() actionHandlerMap {
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
			s.handleRawN8NAction(actionID, w, r)
		}
	}
	return out
}

func (s *server) handleRawN8NAction(actionID string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.n8nActionInvoker == nil {
		writeError(w, http.StatusBadGateway, errors.New("n8n action invoker is not configured"))
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, rawN8NActionMaxBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("read n8n action body: %w", err))
		return
	}
	resp, err := s.n8nActionInvoker.InvokeN8NAction(r.Context(), gptplugin.N8NActionCall{
		ActionID: actionID,
		SubPath:  s.actionSubPath(r, actionID),
		RawJSON:  body,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
