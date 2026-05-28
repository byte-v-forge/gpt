package dashboard

import (
	"errors"
	"net/http"

	"orchestrator/pb"
)

type submitJobOTPRequest struct {
	OTP string `json:"otp"`
}

func (s *server) submitJobOTP(w http.ResponseWriter, r *http.Request, jobID string) {
	var req submitJobOTPRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	resp, err := s.otpClient.SubmitOTP(r.Context(), &pb.SubmitOTPRequest{
		JobId: jobID,
		Otp:   req.OTP,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if resp.GetErrorMessage() != "" {
		writeError(w, http.StatusBadRequest, errors.New(resp.GetErrorMessage()))
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) resendJobOTP(w http.ResponseWriter, r *http.Request, jobID string) {
	resp, err := s.otpClient.ResendOTP(r.Context(), &pb.ResendOTPRequest{
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
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) cancelJob(w http.ResponseWriter, r *http.Request, jobID string) {
	var req pb.CancelJobRequest
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.JobId = jobID
	resp, err := s.jobClient.CancelJob(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if resp.GetErrorMessage() != "" {
		writeError(w, http.StatusBadRequest, errors.New(resp.GetErrorMessage()))
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) confirmManualGoPayPayment(w http.ResponseWriter, r *http.Request, jobID string) {
	resp, err := s.gopayAppClient.ConfirmManualGoPayPayment(r.Context(), &pb.ConfirmManualGoPayPaymentRequest{
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
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) confirmManualAddBalance(w http.ResponseWriter, r *http.Request, jobID string) {
	resp, err := s.gopayAppClient.ConfirmManualAddBalance(r.Context(), &pb.ConfirmManualAddBalanceRequest{
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
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) selectGoPayAddBalance(w http.ResponseWriter, r *http.Request, jobID string) {
	var req pb.ConfirmManualAddBalanceRequest
	if err := readProtoJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req.JobId = jobID
	resp, err := s.gopayAppClient.ConfirmManualAddBalance(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if resp.GetErrorMessage() != "" {
		writeError(w, http.StatusBadRequest, errors.New(resp.GetErrorMessage()))
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
