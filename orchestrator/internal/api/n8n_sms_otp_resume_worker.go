package api

import (
	"strings"
	"time"

	smsv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/sms/v1"

	"orchestrator/internal/channelotpwait"
)

func smsN8NChannelOTPResumeWorkerConfig() n8nChannelOTPResumeWorkerConfig[*smsv1.SmsCodeReceivedEvent] {
	return newN8NChannelOTPResumeWorkerConfig(
		channelotpwait.ChannelSMS,
		func() *smsv1.SmsCodeReceivedEvent { return &smsv1.SmsCodeReceivedEvent{} },
		smsChannelOTPEvent,
	)
}

func smsChannelOTPEvent(event *smsv1.SmsCodeReceivedEvent) channelOTPEvent {
	if event == nil || event.GetCode() == nil {
		return channelOTPEvent{}
	}
	activationID := strings.TrimSpace(event.GetOrderId())
	code := channelotpwait.NormalizeCode(event.GetCode().GetValue())
	if activationID == "" || code == "" {
		return channelOTPEvent{}
	}
	return channelOTPEvent{
		Channel:        channelotpwait.ChannelSMS,
		Targets:        []string{activationID},
		Code:           code,
		Source:         "sms",
		ReceivedAtUnix: smsOTPReceivedAt(event),
	}
}

func smsOTPReceivedAt(event *smsv1.SmsCodeReceivedEvent) int64 {
	if event.GetCode() != nil && event.GetCode().GetReceivedAt() != nil {
		return event.GetCode().GetReceivedAt().AsTime().Unix()
	}
	if event.GetContext() != nil && event.GetContext().GetOccurredAt() != nil {
		return event.GetContext().GetOccurredAt().AsTime().Unix()
	}
	return time.Now().Unix()
}
