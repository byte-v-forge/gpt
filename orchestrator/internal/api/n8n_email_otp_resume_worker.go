package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"

	"orchestrator/internal/accountmail"
	"orchestrator/internal/channelotpwait"
	"orchestrator/pb"
)

func emailN8NChannelOTPResumeWorkerConfig() n8nChannelOTPResumeWorkerConfig[*mailboxv1.MailboxEmailReceivedEvent] {
	return newN8NChannelOTPResumeWorkerConfig(
		channelotpwait.ChannelEmail,
		func() *mailboxv1.MailboxEmailReceivedEvent { return &mailboxv1.MailboxEmailReceivedEvent{} },
		emailChannelOTPEvent,
	)
}

func emailChannelOTPEvent(event *mailboxv1.MailboxEmailReceivedEvent) channelOTPEvent {
	if event == nil || event.GetMessage() == nil {
		return channelOTPEvent{}
	}
	message := accountmail.EnrichMessage(event.GetMessage())
	if !accountmail.IsOpenAIMessage(message) {
		return channelOTPEvent{}
	}
	code := channelotpwait.NormalizeCode(accountmail.OTPCode(message))
	if code == "" {
		return channelOTPEvent{}
	}
	emails := emailOTPMessageEmails(message)
	if len(emails) == 0 {
		return channelOTPEvent{}
	}
	receivedAt := message.GetReceivedAtUnix()
	if receivedAt <= 0 {
		receivedAt = time.Now().Unix()
	}
	messageID := strings.TrimSpace(message.GetId())
	return channelOTPEvent{
		Channel:        channelotpwait.ChannelEmail,
		Targets:        emails,
		Code:           code,
		Source:         "mailbox",
		ReceivedAtUnix: receivedAt,
		Metadata:       emailOTPMetadata(message, messageID),
		MessageID:      messageID,
	}
}

func (s *Server) latestMailboxEmailChannelOTP(ctx context.Context, email string, issuedAfterUnix int64) (n8nChannelOTPLatestResult, error) {
	if s == nil || s.otpProjection == nil {
		return n8nChannelOTPLatestResult{}, nil
	}
	message, code, ok, err := s.otpProjection.LatestMailboxSignal(ctx, email, mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_OTP, issuedAfterUnix)
	if err != nil || !ok {
		return n8nChannelOTPLatestResult{Code: channelotpwait.NormalizeCode(code), Source: "mailbox", Found: ok && channelotpwait.NormalizeCode(code) != ""}, err
	}
	message = accountmail.EnrichMessage(message)
	code = channelotpwait.NormalizeCode(code)
	if code == "" {
		code = channelotpwait.NormalizeCode(accountmail.OTPCode(message))
	}
	messageID := strings.TrimSpace(message.GetId())
	return n8nChannelOTPLatestResult{
		Code:           code,
		Source:         "mailbox",
		Found:          code != "",
		ReceivedAtUnix: message.GetReceivedAtUnix(),
		MessageID:      messageID,
		Metadata:       emailOTPMetadata(message, messageID),
	}, nil
}

func (s *Server) requestMailboxEmailChannelOTPPoll(ctx context.Context, req n8nChannelOTPWaitRequest, cfg n8nChannelOTPWaitConfig, timeout time.Duration) error {
	if s.mailboxPollRequester == nil {
		return fmt.Errorf("mailbox poll requester is required")
	}
	return s.mailboxPollRequester.RequestMailboxEmailPoll(ctx, req.Target, mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_UNSPECIFIED, req.OTPIssuedAfter, timeout, cfg.PollReason)
}

func emailOTPMessageEmails(message *mailboxv1.EmailInboxMessage) []string {
	if message == nil {
		return nil
	}
	emails := []string{message.GetMailboxEmail(), message.GetSourceMailboxEmail()}
	emails = append(emails, message.GetRecipients()...)
	return channelotpwait.EmailCandidates(emails...)
}

func emailOTPMetadata(message *mailboxv1.EmailInboxMessage, messageID string) *pb.N8NChannelOTPMetadata {
	if message == nil {
		return nil
	}
	return &pb.N8NChannelOTPMetadata{
		MessageId:   strings.TrimSpace(messageID),
		ProviderKey: strings.TrimSpace(message.GetProviderKey()),
	}
}
