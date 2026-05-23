package activities

import (
	"context"
	"fmt"
	"strings"

	pb "orchestrator/pb"
)

func (s *Server) claimEnvelopeAddBalance(ctx context.Context, step activityStep, envelope *pb.GoPayEnvelopeAddBalance, output *GoPayAppAddBalanceOutput, data map[string]any) (any, error) {
	if s.gopayClient == nil {
		err := fmt.Errorf("gopay-app client not configured")
		data["error_message"] = err.Error()
		return data, err
	}

	envelopeLink := strings.TrimSpace(envelope.GetLink())
	if envelopeLink == "" {
		envelopeLink = s.loadRuntimeSecret(ctx, goPayEnvelopeLinkSecretKey)
	}
	envelopeRequestID := strings.TrimSpace(envelope.GetEnvelopeRequestId())
	data["method"] = "envelope"
	data["status"] = "claiming"
	data["envelope_link_present"] = envelopeLink != ""
	data["envelope_request_id_present"] = envelopeRequestID != ""
	if envelopeLink == "" && envelopeRequestID == "" {
		err := fmt.Errorf("GOPAY_ADD_BALANCE_ENVELOPE_LINK or envelope_request_id is required")
		data["error_message"] = err.Error()
		return data, err
	}
	if envelopeLink != "" {
		if err := s.saveRuntimeSecret(ctx, goPayEnvelopeLinkSecretKey, envelopeLink); err != nil {
			err = fmt.Errorf("store gopay envelope link: %w", err)
			data["error_message"] = err.Error()
			return data, err
		}
	}

	step.progress("claiming gopay envelope", map[string]any{
		"envelope_link_present":       envelopeLink != "",
		"envelope_request_id_present": envelopeRequestID != "",
	})
	resp, err := s.gopayClient.ClaimEnvelope(ctx, &pb.ClaimEnvelopeRequest{
		EnvelopeRequestId: envelopeRequestID,
		Link:              envelopeLink,
		StateJson:         output.GetStateJson(),
	})
	output.StateJson = goPayWorkflowStateAfter(output.GetStateJson(), responseStateJSON(resp))
	data["envelope"] = claimEnvelopeData(resp)
	if err != nil {
		err = fmt.Errorf("ClaimEnvelope: %w", err)
		data["error_message"] = err.Error()
		return data, err
	}
	if resp == nil {
		err := fmt.Errorf("ClaimEnvelope returned empty response")
		data["error_message"] = err.Error()
		return data, err
	}
	output.Success = resp.GetSuccess()
	output.Method = "envelope"
	output.Status = resp.GetStatus()
	data["status"] = resp.GetStatus()
	if !resp.GetSuccess() {
		message := strings.TrimSpace(resp.GetErrorMessage())
		if message == "" {
			message = "claim envelope failed"
		}
		output.ErrorMessage = message
		err := fmt.Errorf("ClaimEnvelope: %s", message)
		data["error_message"] = err.Error()
		return data, err
	}
	data["add_balance_complete"] = true
	return data, nil
}

func claimEnvelopeData(resp *pb.ClaimEnvelopeResponse) map[string]any {
	if resp == nil {
		return map[string]any{"response_present": false}
	}
	return map[string]any{
		"response_present":             true,
		"success":                      resp.GetSuccess(),
		"error_message":                resp.GetErrorMessage(),
		"envelope_request_id":          resp.GetEnvelopeRequestId(),
		"response_envelope_request_id": resp.GetResponseEnvelopeRequestId(),
		"status":                       resp.GetStatus(),
		"http_status":                  resp.GetHttpStatus(),
		"raw_json":                     limitStepText(resp.GetRawJson(), 2000),
	}
}
