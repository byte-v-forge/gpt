package activities

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"

	"orchestrator/internal/protowrap"
	"orchestrator/pb"
)

const activityHeartbeatInterval = 5 * time.Second

func recordActivityProgress(ctx context.Context, jobID, step, message string, fields proto.Message) {
}

func recordActivityProgressEvery(ctx context.Context, last *time.Time, jobID, step, message string, fields proto.Message) {
	if last != nil && !last.IsZero() && time.Since(*last) < activityHeartbeatInterval {
		return
	}
	recordActivityProgress(ctx, jobID, step, message, fields)
	if last != nil {
		*last = time.Now()
	}
}

func startActivityHeartbeat(ctx context.Context, jobID, step, message string, fields proto.Message) func() {
	done := make(chan struct{})
	snapshot := cloneActivityProgressFields(fields)
	go func() {
		ticker := time.NewTicker(activityHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				recordActivityProgress(ctx, jobID, step, message, snapshot)
			case <-ctx.Done():
				return
			case <-done:
				return
			}
		}
	}()
	return func() {
		close(done)
	}
}

func cloneActivityProgressFields(fields proto.Message) proto.Message {
	if fields == nil {
		return nil
	}
	return proto.Clone(fields)
}

func (s *Server) recordActivityProgress(ctx context.Context, jobID, step, message string, fields proto.Message) {
	recordActivityProgress(ctx, jobID, step, message, fields)
	if s == nil || s.jobStore == nil || jobID == "" || step == "" {
		return
	}
	s.updateRunningStepData(ctx, jobID, step, activityProgressData(message, fields))
}

func (s *Server) recordActivityProgressEvery(ctx context.Context, last *time.Time, jobID, step, message string, fields proto.Message) {
	if last != nil && !last.IsZero() && time.Since(*last) < activityHeartbeatInterval {
		return
	}
	s.recordActivityProgress(ctx, jobID, step, message, fields)
	if last != nil {
		*last = time.Now()
	}
}

func activityProgressData(message string, fields proto.Message) *pb.ActivityProgressData {
	at := time.Now().Unix()
	return &pb.ActivityProgressData{
		ProgressMessage: message,
		ProgressAtUnix:  at,
		Progress: &pb.ActivityProgressSnapshotData{
			Message: message,
			AtUnix:  at,
			Fields:  activityProgressFields(fields),
		},
	}
}

func activityProgressFields(fields proto.Message) *pb.ActivityProgressFields {
	out := &pb.ActivityProgressFields{}
	if !protowrap.SetMessage(out, fields) {
		return nil
	}
	return out
}
