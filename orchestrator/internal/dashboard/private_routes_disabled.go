//go:build !private_plugins

package dashboard

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/byte-v-forge/gpt/pkg/gptplugin"
)

const (
	goPayPaymentActionID             = "GOPAY_PAYMENT"
	goPayQRISPaymentActivateActionID = "GOPAY_QRIS_PAYMENT_ACTIVATE"
	goPayWAPaymentActionID           = "GOPAY_WA_PAYMENT"
	goPayPaymentRebindActionID       = "GOPAY_PAYMENT_REBIND"
)

func (s *server) privateRouteBindings() []routeBinding {
	return []routeBinding{
		{"/workflows/gopay-payment", s.goPayWorkflowStartHandler(goPayPaymentActionID)},
		{"/workflows/gopay-qris-payment-activate", s.goPayWorkflowStartHandler(goPayQRISPaymentActivateActionID)},
		{"/actions/gopay-payment/", s.goPayActionHandler(goPayPaymentActionID, "/actions/gopay-payment/")},
		{"/actions/gopay-qris-payment-activate/", s.goPayActionHandler(goPayQRISPaymentActivateActionID, "/actions/gopay-qris-payment-activate/")},
		{"/actions/gopay-wa-payment/", s.goPayActionHandler(goPayWAPaymentActionID, "/actions/gopay-wa-payment/")},
		{"/actions/gopay-payment-rebind/", s.goPayActionHandler(goPayPaymentRebindActionID, "/actions/gopay-payment-rebind/")},
	}
}

func (s *server) goPayWorkflowStartHandler(actionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.n8nActions == nil {
			writeError(w, http.StatusBadGateway, errors.New("gopay n8n workflow starter is not configured"))
			return
		}
		workflow := newN8NWebhookClient(goPayWorkflowName(actionID), s.n8nWebhookBaseURL, goPayWorkflowWebhookPath(actionID))
		if workflow == nil {
			writeError(w, http.StatusBadGateway, fmt.Errorf("n8n %s workflow is not configured", goPayWorkflowName(actionID)))
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, rawN8NActionMaxBodyBytes))
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("read gopay n8n workflow body: %w", err))
			return
		}
		started, err := s.n8nActions.StartN8NWorkflow(r.Context(), gptplugin.N8NWorkflowStartCall{
			ActionID: actionID,
			RawJSON:  body,
		})
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		if err := workflow.trigger(r.Context(), started.TriggerPayload); err != nil {
			_ = s.n8nActions.FailN8NWorkflowTrigger(r.Context(), gptplugin.N8NWorkflowTriggerFailure{
				ActionID:     actionID,
				JobID:        started.JobID,
				AccountID:    started.AccountID,
				ErrorMessage: err.Error(),
				Data:         n8nTriggerFailedMap(),
			})
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeRawN8NWorkflowStartedJSON(w, started.Response)
	}
}

func goPayWorkflowName(actionID string) string {
	switch actionID {
	case goPayQRISPaymentActivateActionID:
		return "gopay-qris-payment-activate"
	default:
		return "gopay-payment"
	}
}

func goPayWorkflowWebhookPath(actionID string) string {
	switch actionID {
	case goPayQRISPaymentActivateActionID:
		return "gpt/gopay-qris-payment-activate"
	default:
		return "gpt/gopay-payment"
	}
}

func (s *server) goPayActionHandler(actionID string, prefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.n8nActions == nil {
			writeError(w, http.StatusBadGateway, errors.New("gopay n8n action invoker is not configured"))
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, rawN8NActionMaxBodyBytes))
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("read gopay n8n action body: %w", err))
			return
		}
		resp, err := s.n8nActions.InvokeN8NAction(r.Context(), gptplugin.N8NActionCall{
			ActionID: actionID,
			SubPath:  strings.Trim(strings.TrimPrefix(r.URL.Path, strings.TrimRight(prefix, "/")+"/"), "/"),
			RawJSON:  body,
		})
		writeN8NAction(w, resp, err)
	}
}

func (s *server) handlePrivateJobAction(http.ResponseWriter, *http.Request, string, []string) bool {
	return false
}
