package jobqueue

import (
	"strings"

	"github.com/byte-v-forge/common-lib/eventbus"
	"github.com/byte-v-forge/common-lib/eventcatalog"

	"orchestrator/pb"
)

const sourceService = "gpt-service"

func ActionRequestedMessage(jobID string, action string, accountID string, reason string) (eventbus.Message, bool) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return eventbus.Message{}, false
	}
	action = strings.ToUpper(strings.TrimSpace(action))
	accountID = strings.TrimSpace(accountID)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "job_created"
	}
	eventID := eventbus.StableEventID("gpt-job-action-", jobID, action)
	return eventbus.Message{
		Subject: eventcatalog.GPTJobActionRequested.Subject,
		Event: &pb.GPTJobActionRunRequest{
			JobId:     jobID,
			Action:    action,
			AccountId: accountID,
			Reason:    reason,
		},
		Context: eventbus.NewEventContext(eventbus.EventContextConfig{
			EventID:       eventID,
			EventName:     eventcatalog.GPTJobActionRequested.EventName,
			SourceService: sourceService,
			CorrelationID: jobID,
		}),
		Attributes: eventbus.Attributes(
			"job_id", jobID,
			"action", action,
			"account_id", accountID,
			"reason", reason,
		),
	}, true
}
