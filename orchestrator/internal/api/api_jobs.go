package api

import (
	"context"
	"orchestrator/internal/jobprojection"
	"orchestrator/pb"
	"strings"
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

func (s *Server) WatchJob(req *pb.WatchJobRequest, stream pb.JobService_WatchJobServer) error {
	jobID := strings.TrimSpace(req.GetJobId())
	if jobID == "" {
		return stream.Send(&pb.WatchJobResponse{ErrorMessage: "job_id is required"})
	}
	if _, err := s.jobStore.GetSnapshot(stream.Context(), jobID); err != nil {
		return stream.Send(&pb.WatchJobResponse{ErrorMessage: err.Error()})
	}

	return s.watchJobEvents(stream.Context(), []string{jobID}, "", func(event *pb.JobEvent) (bool, error) {
		if event == nil {
			return true, nil
		}
		if err := stream.Send(&pb.WatchJobResponse{Event: event}); err != nil {
			return false, err
		}
		return !snapshotIsTerminal(event.GetSnapshot()), nil
	})
}

func (s *Server) WatchJobs(req *pb.WatchJobsRequest, stream pb.JobService_WatchJobsServer) error {
	return s.watchJobEvents(stream.Context(), req.GetJobIds(), req.GetStatus(), func(event *pb.JobEvent) (bool, error) {
		if event == nil {
			return true, nil
		}
		if err := stream.Send(&pb.WatchJobsResponse{Event: event}); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (s *Server) watchJobEvents(ctx context.Context, jobIDs []string, status string, send func(*pb.JobEvent) (bool, error)) error {
	if s.jobEvents == nil {
		_, err := send(nil)
		return err
	}
	ch, cancel := s.jobEvents.Subscribe(ctx)
	defer cancel()

	jobIDs = compactJobIDs(jobIDs)
	status = strings.ToUpper(strings.TrimSpace(status))
	lastSent := int64(0)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-ch:
			if !ok {
				return nil
			}
			if event == nil || event.GetEventId() <= lastSent || !jobEventMatches(event, jobIDs, status) {
				continue
			}
			keepGoing, err := send(event)
			if err != nil || !keepGoing {
				return err
			}
			lastSent = event.GetEventId()
		}
	}
}

func jobEventMatches(event *pb.JobEvent, jobIDs []string, status string) bool {
	if event == nil {
		return false
	}
	if len(jobIDs) > 0 {
		matched := false
		for _, jobID := range jobIDs {
			if event.GetJobId() == jobID {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return status == "" || strings.EqualFold(event.GetSnapshot().GetJob().GetStatus(), status)
}

func compactJobIDs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func snapshotIsTerminal(snapshot *pb.JobSnapshot) bool {
	if snapshot == nil || snapshot.GetJob() == nil {
		return false
	}
	status := strings.ToUpper(strings.TrimSpace(snapshot.GetJob().GetStatus()))
	return status == "SUCCEEDED" ||
		status == "FAILED_RETRYABLE" ||
		status == "FAILED_RECOVERABLE" ||
		status == "FAILED_FINAL"
}
