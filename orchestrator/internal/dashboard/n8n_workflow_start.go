package dashboard

import (
	"context"
	"errors"
	"net/http"

	"google.golang.org/protobuf/proto"

	"orchestrator/internal/contracts"
	"orchestrator/pb"
)

const n8nTriggerFailedReason = "n8n_trigger_failed"

type n8nStartedJobResponse interface {
	startedResponse
	GetJobId() string
}

type n8nWorkflowStartConfig[Req any, Resp n8nStartedJobResponse] struct {
	ActionID       string
	WorkflowLabel  string
	APILabel       string
	API            any
	Decode         func(*http.Request) (Req, error)
	Start          func(context.Context, Req) (Resp, string, error)
	TriggerPayload func(Req, Resp, string) proto.Message
	Fail           func(context.Context, Req, Resp, string, error)
}

type n8nWorkflowAccountStartCall[API any, Req any, Resp n8nStartedJobResponse] func(API, context.Context, string, Req) (Resp, string, error)
type n8nWorkflowStartFailureCall[API any, Resp n8nStartedJobResponse] func(API, context.Context, string, Resp, string, error)

func handleN8NWorkflowStart[Req any, Resp n8nStartedJobResponse](s *server, w http.ResponseWriter, r *http.Request, cfg n8nWorkflowStartConfig[Req, Resp]) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	req, err := cfg.Decode(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	workflow := s.n8nWorkflow(cfg.ActionID)
	if workflow == nil {
		writeError(w, http.StatusBadGateway, n8nWorkflowUnavailableError(cfg.WorkflowLabel))
		return
	}
	if cfg.API == nil {
		writeError(w, http.StatusBadGateway, n8nWorkflowAPIUnavailableError(cfg.APILabel))
		return
	}
	resp, accountID, err := cfg.Start(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	payload := n8nAccountWorkflowPayload(req, resp, accountID)
	if cfg.TriggerPayload != nil {
		payload = cfg.TriggerPayload(req, resp, accountID)
	}
	if err := workflow.trigger(r.Context(), payload); err != nil {
		if cfg.Fail != nil {
			cfg.Fail(r.Context(), req, resp, accountID, err)
		}
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeStartedJSON(w, resp)
}

func n8nWorkflowStartHandler[Req any, Resp n8nStartedJobResponse](s *server, cfg n8nWorkflowStartConfig[Req, Resp]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { handleN8NWorkflowStart(s, w, r, cfg) }
}

func n8nProtoJSONWorkflowStartRequest[T proto.Message](newRequest func() T) func(*http.Request) (T, error) {
	return func(r *http.Request) (T, error) {
		req := newRequest()
		if err := readProtoJSON(r, req); err != nil {
			return req, err
		}
		return req, nil
	}
}

func n8nAccountWorkflowPayload[Req any, Resp n8nStartedJobResponse](_ Req, resp Resp, accountID string) proto.Message {
	return &pb.N8NWorkflowTriggerPayload{
		JobId:     resp.GetJobId(),
		AccountId: accountID,
	}
}

func n8nTriggerFailedMap() map[string]any {
	return map[string]any{"reason": n8nTriggerFailedReason}
}

func n8nWorkflowStartConfigFor[API any, Req any, Resp n8nStartedJobResponse](profile contracts.ActionWorkflowStartProfile, api API, decode func(*http.Request) (Req, error), start n8nWorkflowAccountStartCall[API, Req, Resp], payload func(Req, Resp, string) proto.Message, fail n8nWorkflowStartFailureCall[API, Resp]) n8nWorkflowStartConfig[Req, Resp] {
	return n8nWorkflowStartConfig[Req, Resp]{
		ActionID:       profile.ActionID,
		WorkflowLabel:  profile.WorkflowLabel,
		APILabel:       profile.APILabel,
		API:            api,
		Decode:         decode,
		TriggerPayload: payload,
		Start: func(ctx context.Context, req Req) (Resp, string, error) {
			return start(api, ctx, profile.ActionID, req)
		},
		Fail: func(ctx context.Context, _ Req, resp Resp, accountID string, err error) {
			if fail != nil {
				fail(api, ctx, profile.ActionID, resp, accountID, err)
			}
		},
	}
}

func n8nWorkflowUnavailableError(label string) error {
	if label = firstNonEmptyString(label); label != "" {
		return errors.New("n8n " + label + " workflow is not configured")
	}
	return errors.New("n8n workflow is not configured")
}

func n8nWorkflowAPIUnavailableError(label string) error {
	if label = firstNonEmptyString(label); label != "" {
		return errors.New("n8n " + label + " action API is not configured")
	}
	return errors.New("n8n action API is not configured")
}
