package api

import (
	"context"
	"fmt"
	"strings"

	"orchestrator/internal/jobdata"
	"orchestrator/internal/jobprojection"
	"orchestrator/internal/jobstatus"
	"orchestrator/pb"
)

func (s *Server) GetJob(ctx context.Context, req *pb.GetJobRequest) (*pb.GetJobResponse, error) {
	jobID := strings.TrimSpace(req.GetJobId())
	if jobID == "" {
		return &pb.GetJobResponse{ErrorMessage: "job_id is required"}, nil
	}

	snapshot, err := s.jobStore.GetSnapshot(ctx, jobID)
	if err != nil {
		return &pb.GetJobResponse{ErrorMessage: err.Error()}, nil
	}

	return &pb.GetJobResponse{Snapshot: snapshot}, nil
}

func (s *Server) ListJobs(ctx context.Context, req *pb.ListJobsRequest) (*pb.ListJobsResponse, error) {
	before := req.GetBefore()
	snapshots, next, hasMore, err := s.jobStore.ListSnapshots(ctx, jobprojection.ListFilter{
		Limit:           int(req.GetLimit()),
		Status:          req.GetStatus(),
		Action:          req.GetAction(),
		AccountID:       req.GetAccountId(),
		BeforeUpdatedAt: before.GetUpdatedAt(),
		BeforeJobID:     before.GetJobId(),
	})
	if err != nil {
		return &pb.ListJobsResponse{ErrorMessage: err.Error()}, nil
	}

	return &pb.ListJobsResponse{Snapshots: snapshots, Next: next, HasMore: hasMore}, nil
}

func (s *Server) CancelJob(ctx context.Context, req *pb.CancelJobRequest) (*pb.CancelJobResponse, error) {
	jobID := strings.TrimSpace(req.GetJobId())
	if jobID == "" {
		return &pb.CancelJobResponse{ErrorMessage: "job_id is required"}, nil
	}
	job, err := s.getJob(ctx, jobID)
	if err != nil {
		return &pb.CancelJobResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	if strings.EqualFold(job.Status, jobstatus.Canceled) {
		snapshot, _ := s.jobStore.GetSnapshot(ctx, jobID)
		return &pb.CancelJobResponse{Success: true, JobId: jobID, Snapshot: snapshot}, nil
	}
	if !strings.EqualFold(job.Status, jobstatus.Running) {
		return &pb.CancelJobResponse{JobId: jobID, ErrorMessage: "job is not running: " + job.Status}, nil
	}
	reason := strings.TrimSpace(req.GetReason())
	if reason == "" {
		reason = "manual job cancel"
	}
	cancelData := jobdata.Message(&pb.JobCancelResultData{
		Canceled: true,
		Reason:   reason,
		Engine:   "n8n",
	})
	if err := s.jobStore.Cancel(ctx, jobID, reason, cancelData); err != nil {
		return &pb.CancelJobResponse{JobId: jobID, ErrorMessage: err.Error()}, nil
	}
	snapshot, err := s.jobStore.GetSnapshot(ctx, jobID)
	if err != nil {
		return &pb.CancelJobResponse{JobId: jobID, ErrorMessage: fmt.Sprintf("load canceled job: %v", err)}, nil
	}
	return &pb.CancelJobResponse{Success: true, JobId: jobID, Snapshot: snapshot}, nil
}
