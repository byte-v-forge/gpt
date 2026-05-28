package api

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/eventbus"
	"gorm.io/gorm"

	"orchestrator/internal/jobprojection"
	"orchestrator/pb"
)

const (
	jobActionWorkerID          = "gpt-job-action-worker"
	jobActionClaimLeaseSeconds = int32(300)
	jobActionRetryDelay        = 5 * time.Second
	jobActionBusyDelay         = 30 * time.Second
)

type jobActionWorker struct {
	server *Server
}

func RunJobActionWorker(ctx context.Context, consumer eventbus.Consumer, server *Server) error {
	worker := &jobActionWorker{server: server}
	return eventbus.RunConsumerWorker(ctx, eventbus.ConsumerWorkerConfig{
		Name:     "GPT job action requests",
		Consumer: consumer,
		Handler:  worker.handle,
		Logf:     logJobActionWorker,
	})
}

func (w *jobActionWorker) handle(ctx context.Context, message eventbus.ReceivedMessage) {
	request, ok := decodeJobActionRunRequest(message)
	if !ok {
		eventbus.TermMessage(ctx, message, "terminate malformed GPT job action request", logJobActionWorker)
		return
	}
	jobID := strings.TrimSpace(request.GetJobId())
	claim, terminal, err := w.server.jobStore.ClaimJob(ctx, jobID, jobActionWorkerID, jobActionClaimLeaseSeconds, jobActionRunnableActions, jobActionStaleSteps)
	if err != nil {
		w.handleClaimError(ctx, message, jobID, err)
		return
	}
	if terminal {
		eventbus.AckMessage(ctx, message, "ack terminal GPT job action request", logJobActionWorker)
		return
	}
	if claim == nil || claim.Job == nil {
		eventbus.TermMessage(ctx, message, "terminate empty GPT job action claim", logJobActionWorker)
		return
	}
	response, err := w.server.RunGPTJobAction(ctx, &pb.GPTJobActionRequest{
		JobId:          claim.Job.ID,
		Action:         claim.Job.Action,
		AccountId:      claim.Job.AccountID,
		RunId:          claim.Job.ID,
		CorrelationId:  claim.Job.ID,
		IdempotencyKey: jobprojection.IdempotencyKey(claim.Job.ID, claim.AttemptCount),
		Params:         claim.Params,
	})
	if err != nil {
		log.Printf("[orchestrator] run GPT job action failed job=%s action=%s: %v", claim.Job.ID, claim.Job.Action, err)
		eventbus.NakMessageDelay(ctx, message, jobActionRetryDelay, "retry GPT job action request", logJobActionWorker)
		return
	}
	if response != nil && response.GetErrorMessage() != "" {
		log.Printf("[orchestrator] GPT job action completed with error job=%s action=%s status=%s: %s", response.GetJobId(), response.GetAction(), response.GetStatus(), response.GetErrorMessage())
	}
	eventbus.AckMessage(ctx, message, "ack GPT job action request", logJobActionWorker)
}

func (w *jobActionWorker) handleClaimError(ctx context.Context, message eventbus.ReceivedMessage, jobID string, err error) {
	switch {
	case errors.Is(err, jobprojection.ErrJobAlreadyRunning):
		eventbus.NakMessageDelay(ctx, message, jobActionBusyDelay, "delay busy GPT job action request", logJobActionWorker)
	case errors.Is(err, jobprojection.ErrJobUnsupported), errors.Is(err, jobprojection.ErrJobNotClaimed), errors.Is(err, gorm.ErrRecordNotFound):
		log.Printf("[orchestrator] terminate invalid GPT job action request job=%s: %v", jobID, err)
		eventbus.TermMessage(ctx, message, "terminate invalid GPT job action request", logJobActionWorker)
	default:
		log.Printf("[orchestrator] claim GPT job action failed job=%s: %v", jobID, err)
		eventbus.NakMessageDelay(ctx, message, jobActionRetryDelay, "retry GPT job action claim", logJobActionWorker)
	}
}

func decodeJobActionRunRequest(message eventbus.ReceivedMessage) (*pb.GPTJobActionRunRequest, bool) {
	request := &pb.GPTJobActionRunRequest{}
	if err := eventbus.UnmarshalPayload(message, request); err != nil {
		log.Printf("[orchestrator] decode GPT job action request failed event_id=%s: %v", eventbus.EventID(message), err)
		return nil, false
	}
	return request, strings.TrimSpace(request.GetJobId()) != ""
}

func logJobActionWorker(format string, args ...any) {
	log.Printf("[orchestrator] "+format, args...)
}
