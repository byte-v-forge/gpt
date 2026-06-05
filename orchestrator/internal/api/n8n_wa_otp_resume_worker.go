package api

import (
	"context"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/eventcatalog"
	wav1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/wa/v1"

	"orchestrator/internal/channelotpwait"
)

func waN8NChannelOTPResumeWorkerConfig() n8nChannelOTPResumeWorkerConfig[*wav1.WaOtpReceivedEvent] {
	return newN8NChannelOTPResumeWorkerConfig(
		channelotpwait.ChannelWA,
		eventcatalog.WAOTPReceived,
		func() *wav1.WaOtpReceivedEvent { return &wav1.WaOtpReceivedEvent{} },
		waChannelOTPEvent,
	)
}

func waChannelOTPEvent(_ context.Context, _ *Server, event *wav1.WaOtpReceivedEvent) (channelOTPEvent, error) {
	if event == nil {
		return channelOTPEvent{}, nil
	}
	number := channelotpwait.NormalizeNumber(event.GetE164Number())
	code := channelotpwait.NormalizeCode(event.GetOtp())
	if number == "" || code == "" {
		return channelOTPEvent{}, nil
	}
	return channelOTPEvent{
		Channel:        channelotpwait.ChannelWA,
		Targets:        []string{number},
		Code:           code,
		Source:         waOTPSource(event),
		ReceivedAtUnix: waOTPReceivedAt(event),
	}, nil
}

func waOTPReceivedAt(event *wav1.WaOtpReceivedEvent) int64 {
	if event.GetReceivedAt() != nil {
		return event.GetReceivedAt().AsTime().Unix()
	}
	if event.GetMetadata() != nil && event.GetMetadata().GetTime() != nil {
		return event.GetMetadata().GetTime().AsTime().Unix()
	}
	return time.Now().Unix()
}

func waOTPSource(event *wav1.WaOtpReceivedEvent) string {
	if event == nil || event.GetSource() == wav1.WaOtpSource_WA_OTP_SOURCE_UNSPECIFIED {
		return "wa"
	}
	return strings.ToLower(strings.TrimPrefix(event.GetSource().String(), "WA_OTP_SOURCE_"))
}
