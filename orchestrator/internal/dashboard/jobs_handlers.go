package dashboard

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/byte-v-forge/common-lib/httpx"

	"orchestrator/pb"
)

func (s *server) handleJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	resp, err := s.workflowAPI.ListJobs(r.Context(), &pb.ListJobsRequest{
		Limit:     int32(httpx.QueryInt(r, "limit", 100)),
		Status:    strings.TrimSpace(r.URL.Query().Get("status")),
		Action:    strings.TrimSpace(r.URL.Query().Get("action")),
		AccountId: strings.TrimSpace(r.URL.Query().Get("account_id")),
		Before:    requestJobListCursor(r),
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if resp.GetErrorMessage() != "" {
		writeError(w, http.StatusBadGateway, errors.New(resp.GetErrorMessage()))
		return
	}
	writeProtoJSON(w, http.StatusOK, resp)
}

func (s *server) handleJob(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/jobs/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		writeError(w, http.StatusBadRequest, errors.New("job_id is required"))
		return
	}
	jobID := strings.TrimSpace(parts[0])

	if len(parts) > 1 {
		if s.handlePrivateJobAction(w, r, jobID, parts) {
			return
		}
		switch parts[1] {
		case "otp":
			if len(parts) == 3 && parts[2] == "resend" {
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				s.resendJobOTP(w, r, jobID)
				return
			}
			writeError(w, http.StatusNotFound, fmt.Errorf("unsupported job otp action: %s", strings.Join(parts[1:], "/")))
			return
		case "cancel":
			if len(parts) != 2 {
				writeError(w, http.StatusNotFound, fmt.Errorf("unsupported job cancel action: %s", strings.Join(parts[1:], "/")))
				return
			}
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			s.cancelJob(w, r, jobID)
			return
		default:
			writeError(w, http.StatusNotFound, fmt.Errorf("unsupported job action: %s", parts[1]))
			return
		}
	}

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	resp, err := s.workflowAPI.GetJob(r.Context(), &pb.GetJobRequest{JobId: jobID})
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if resp.GetErrorMessage() != "" {
		writeError(w, http.StatusBadGateway, errors.New(resp.GetErrorMessage()))
		return
	}
	writeProtoJSON(w, http.StatusOK, resp)
}

func requestJobListCursor(r *http.Request) *pb.JobListCursor {
	updatedAt := int64(httpx.QueryInt(r, "before_updated_at", 0))
	jobID := strings.TrimSpace(r.URL.Query().Get("before_job_id"))
	if updatedAt <= 0 && jobID == "" {
		return nil
	}
	return &pb.JobListCursor{UpdatedAt: updatedAt, JobId: jobID}
}
