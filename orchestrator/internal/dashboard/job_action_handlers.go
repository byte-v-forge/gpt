package dashboard

import (
	"errors"
	"net/http"

	"orchestrator/pb"
)

func (s *server) submitChannelOTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req pb.SubmitOTPRequest
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	resp, err := s.workflowAPI.SubmitOTP(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if resp.GetErrorMessage() != "" {
		writeError(w, http.StatusBadRequest, errors.New(resp.GetErrorMessage()))
		return
	}
	writeProtoJSON(w, http.StatusOK, resp)
}

func (s *server) resendJobOTP(w http.ResponseWriter, r *http.Request, jobID string) {
	resp, err := s.workflowAPI.ResendOTP(r.Context(), &pb.ResendOTPRequest{
		JobId: jobID,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if resp.GetErrorMessage() != "" {
		writeError(w, http.StatusBadRequest, errors.New(resp.GetErrorMessage()))
		return
	}
	writeProtoJSON(w, http.StatusOK, resp)
}

func (s *server) cancelJob(w http.ResponseWriter, r *http.Request, jobID string) {
	var req pb.CancelJobRequest
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.JobId = jobID
	resp, err := s.workflowAPI.CancelJob(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if resp.GetErrorMessage() != "" {
		writeError(w, http.StatusBadRequest, errors.New(resp.GetErrorMessage()))
		return
	}
	writeProtoJSON(w, http.StatusOK, resp)
}
