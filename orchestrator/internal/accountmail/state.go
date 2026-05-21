package accountmail

import (
	"strings"

	"orchestrator/pb"
)

const (
	StatusActivated   = "ACTIVATED"
	StatusDeactivated = "DEACTIVATED"
)

type State struct {
	Status       string
	ErrorMessage string
	PlusActive   bool
}

type OTP struct {
	Code           string
	Subject        string
	ReceivedAtUnix int64
}

func DetectState(account *pb.Account, messages []*pb.EmailInboxMessage) State {
	for _, message := range MessagesForAccount(account, messages) {
		text := strings.ToLower(strings.Join([]string{
			message.GetSubject(),
			message.GetBodyPreview(),
			message.GetBodyText(),
			message.GetHtmlBody(),
		}, "\n"))
		switch {
		case containsAny(text, "deactivated", "disabled", "account has been deactivated", "账号已停用", "账户已停用", "账号失效", "账户失效"):
			return State{Status: StatusDeactivated, ErrorMessage: "mailbox event: account deactivated"}
		case containsAny(text, "new plan", "chatgpt plus", "plus plan", "subscription", "subscribed", "your plan has changed", "plus 已激活"):
			return State{Status: StatusActivated, PlusActive: true}
		}
	}
	return State{}
}

func LatestMessageUnix(account *pb.Account, messages []*pb.EmailInboxMessage) int64 {
	var latest int64
	for _, message := range MessagesForAccount(account, messages) {
		if message.GetReceivedAtUnix() > latest {
			latest = message.GetReceivedAtUnix()
		}
	}
	return latest
}

func LatestOTP(account *pb.Account, messages []*pb.EmailInboxMessage) OTP {
	var latest OTP
	for _, message := range MessagesForAccount(account, messages) {
		code := otpCode(message)
		if code == "" || message.GetReceivedAtUnix() < latest.ReceivedAtUnix {
			continue
		}
		latest = OTP{
			Code:           code,
			Subject:        message.GetSubject(),
			ReceivedAtUnix: message.GetReceivedAtUnix(),
		}
	}
	return latest
}

func InboxMessages(account *pb.Account, inbox *pb.FetchMailboxInboxesResponse) []*pb.EmailInboxMessage {
	if inbox == nil {
		return nil
	}
	out := []*pb.EmailInboxMessage{}
	for _, result := range inbox.GetResults() {
		out = append(out, result.GetMessages()...)
	}
	return MessagesForAccount(account, out)
}

func MessagesForAccount(account *pb.Account, messages []*pb.EmailInboxMessage) []*pb.EmailInboxMessage {
	out := []*pb.EmailInboxMessage{}
	for _, message := range messages {
		if MessageMatches(account, message) {
			out = append(out, message)
		}
	}
	return out
}

func MessageMatches(account *pb.Account, message *pb.EmailInboxMessage) bool {
	accountEmail := NormalizeEmail(account.GetEmail())
	if accountEmail == "" || message == nil {
		return false
	}
	for _, recipient := range message.GetRecipients() {
		if NormalizeEmail(recipient) == accountEmail {
			return true
		}
	}
	return NormalizeEmail(message.GetMailboxEmail()) == accountEmail
}

func PrimaryMailbox(account *pb.Account) string {
	if account == nil {
		return ""
	}
	if value := NormalizeEmail(account.GetPrimaryMailboxEmail()); value != "" {
		return value
	}
	return CanonicalEmail(account.GetEmail())
}

func CanonicalEmail(email string) string {
	normalized := NormalizeEmail(email)
	local, domain, ok := strings.Cut(normalized, "@")
	if !ok || local == "" || domain == "" {
		return normalized
	}
	local, _, _ = strings.Cut(local, "+")
	return local + "@" + domain
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func otpCode(message *pb.EmailInboxMessage) string {
	if message == nil {
		return ""
	}
	if signal := message.GetPrimarySignal(); signal.GetKind() == pb.EmailSignalKind_EMAIL_SIGNAL_KIND_OTP {
		if code := strings.TrimSpace(signal.GetCode()); code != "" {
			return code
		}
	}
	for _, signal := range message.GetSignals() {
		if signal.GetKind() == pb.EmailSignalKind_EMAIL_SIGNAL_KIND_OTP {
			if code := strings.TrimSpace(signal.GetCode()); code != "" {
				return code
			}
		}
	}
	return ""
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}
