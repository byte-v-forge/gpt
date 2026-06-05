package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/eventcatalog"
	smsv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/sms/v1"

	"orchestrator/internal/channelotpwait"
	"orchestrator/internal/smsotp"
)

func smsN8NChannelOTPResumeWorkerConfig() n8nChannelOTPResumeWorkerConfig[*smsv1.SmsCodeReceivedEvent] {
	return newN8NChannelOTPResumeWorkerConfig(
		channelotpwait.ChannelSMS,
		eventcatalog.SMSCodeReceived,
		func() *smsv1.SmsCodeReceivedEvent { return &smsv1.SmsCodeReceivedEvent{} },
		smsChannelOTPEvent,
	)
}

func smsChannelOTPEvent(ctx context.Context, server *Server, event *smsv1.SmsCodeReceivedEvent) (channelOTPEvent, error) {
	if event == nil || event.GetCode() == nil {
		return channelOTPEvent{}, nil
	}
	activationID := strings.TrimSpace(event.GetOrderId())
	secretRef := event.GetCode().GetSecretRef()
	if activationID == "" || secretRef.GetSecretId() == "" {
		return channelOTPEvent{}, nil
	}
	if server == nil || server.smsCodeResolver == nil {
		return channelOTPEvent{}, fmt.Errorf("sms code secret resolver is required")
	}
	code, err := server.smsCodeResolver.ResolveCode(ctx, activationID, secretRef)
	if err != nil {
		if !smsotp.Retryable(err) {
			return channelOTPEvent{}, nil
		}
		return channelOTPEvent{}, err
	}
	code = channelotpwait.NormalizeCode(code)
	if code == "" {
		return channelOTPEvent{}, nil
	}
	return channelOTPEvent{
		Channel:        channelotpwait.ChannelSMS,
		Targets:        []string{activationID},
		Code:           code,
		Source:         "sms",
		ReceivedAtUnix: smsOTPReceivedAt(event),
	}, nil
}

func smsOTPReceivedAt(event *smsv1.SmsCodeReceivedEvent) int64 {
	if event.GetCode() != nil && event.GetCode().GetReceivedAt() != nil {
		return event.GetCode().GetReceivedAt().AsTime().Unix()
	}
	if event.GetMetadata() != nil && event.GetMetadata().GetTime() != nil {
		return event.GetMetadata().GetTime().AsTime().Unix()
	}
	return time.Now().Unix()
}
