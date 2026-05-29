package api

import (
	"context"
	"errors"

	"google.golang.org/protobuf/types/known/structpb"

	"orchestrator/internal/resultdata"
	"orchestrator/pb"
)

func (s *Server) markActionFailed(ctx context.Context, jobID string, step string, statusValue string, recoverable bool, retryable bool, err error, data map[string]any) error {
	if err == nil {
		return nil
	}
	markErr := s.activities.MarkJobFailedActivity(ctx, pb.JobFailureInput{
		JobId:        jobID,
		StepName:     step,
		Status:       statusValue,
		Recoverable:  recoverable,
		Retryable:    retryable,
		ErrorMessage: err.Error(),
		Result:       structData(data),
	})
	if markErr != nil {
		return errors.Join(err, markErr)
	}
	return err
}

func structMap(value *structpb.Struct) map[string]any {
	if value == nil {
		return nil
	}
	return value.AsMap()
}

func structData(value map[string]any) *structpb.Struct {
	return resultdata.Struct(value)
}
