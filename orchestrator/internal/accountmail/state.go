package accountmail

import (
	"regexp"
	"strings"

	commonv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/common/v1"
	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"
	"github.com/byte-v-forge/gpt/pkg/gptplugin"

	"orchestrator/internal/channelotpwait"
	"orchestrator/internal/gptaccount"
	"orchestrator/pb"
)

type State struct {
	Status       string
	ErrorMessage string
	PlusActive   bool
	Tier         string
}

type Message struct {
	MailboxEmail string
	FromAddress  string
	Subject      string
	BodyPreview  string
	BodyText     string
	HTMLBody     string
	ReceivedAt   int64
	Recipients   []string
}

var (
	verificationCodePattern = regexp.MustCompile(`(?i)(?:verification|security|login|one[- ]?time|otp|code|验证码|安全代码)[^0-9]{0,80}([0-9]{4,8})`)
	standaloneCodePattern   = regexp.MustCompile(`(^|[^0-9])([0-9]{6})([^0-9]|$)`)
)

func IsOpenAIMessage(message *mailboxv1.EmailInboxMessage) bool {
	if message == nil {
		return false
	}
	if isOpenAIAddress(message.GetFromAddress()) {
		return true
	}
	text := normalizeMailText(strings.Join([]string{
		message.GetSubject(),
		message.GetBodyPreview(),
	}, "\n"))
	return strings.Contains(text, "openai") || strings.Contains(text, "chatgpt")
}

func isOpenAIAddress(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	if start := strings.LastIndex(value, "<"); start >= 0 {
		if end := strings.LastIndex(value, ">"); end > start {
			value = strings.TrimSpace(value[start+1 : end])
		}
	}
	_, domain, ok := strings.Cut(value, "@")
	if !ok {
		domain = value
	}
	domain = strings.Trim(domain, " .>")
	return domain == "openai.com" || strings.HasSuffix(domain, ".openai.com") || domain == "chatgpt.com" || strings.HasSuffix(domain, ".chatgpt.com")
}

func DetectState(account *pb.Account, messages []Message) State {
	for _, message := range MessagesForAccount(account, messages) {
		switch {
		case messageContainsPhrase(message, "Access Deactivated"):
			return State{Status: gptplugin.AccountStatusDeactivated, ErrorMessage: "mailbox event: account deactivated"}
		case messageContainsPhrase(message, "You've successfully subscribed to ChatGPT Plus"):
			return State{Status: gptplugin.AccountStatusActivated, PlusActive: true, Tier: "plus"}
		case messageSubjectEquals(message, "New plan"):
			return State{Status: gptplugin.AccountStatusActivated, PlusActive: true, Tier: "plus"}
		}
	}
	return State{}
}

func LatestMessageUnix(account *pb.Account, messages []Message) int64 {
	var latest int64
	for _, message := range MessagesForAccount(account, messages) {
		if message.ReceivedAt > latest {
			latest = message.ReceivedAt
		}
	}
	return latest
}

func InboxMessages(account *pb.Account, inbox *mailboxv1.FetchMailboxInboxesResponse) []Message {
	if inbox == nil {
		return nil
	}
	out := []Message{}
	for _, result := range inbox.GetResults() {
		out = append(out, MessagesFromPublic(result.GetMessages())...)
	}
	return MessagesForAccount(account, out)
}

func PublicMessagesForAccount(account *pb.Account, messages []*mailboxv1.EmailInboxMessage) []*mailboxv1.EmailInboxMessage {
	out := []*mailboxv1.EmailInboxMessage{}
	for _, message := range messages {
		if message == nil {
			continue
		}
		if MessageMatches(account, messageFromInbox(message)) {
			out = append(out, message)
		}
	}
	return out
}

func MessagesFromPublic(messages []*mailboxv1.EmailInboxMessage) []Message {
	out := make([]Message, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		out = append(out, messageFromInbox(message))
	}
	return out
}

func messageFromInbox(message *mailboxv1.EmailInboxMessage) Message {
	return Message{
		MailboxEmail: message.GetMailboxEmail(),
		FromAddress:  message.GetFromAddress(),
		Subject:      message.GetSubject(),
		BodyPreview:  message.GetBodyPreview(),
		ReceivedAt:   message.GetReceivedAtUnix(),
		Recipients:   append([]string{}, message.GetRecipients()...),
	}
}

func MessagesForAccount(account *pb.Account, messages []Message) []Message {
	out := []Message{}
	for _, message := range messages {
		if MessageMatches(account, message) {
			out = append(out, message)
		}
	}
	return out
}

func MessageMatches(account *pb.Account, message Message) bool {
	accountEmail := NormalizeEmail(gptaccount.Email(account))
	if accountEmail == "" {
		return false
	}
	for _, recipient := range message.Recipients {
		if NormalizeEmail(recipient) == accountEmail {
			return true
		}
	}
	return NormalizeEmail(message.MailboxEmail) == accountEmail
}

func PrimaryMailbox(account *pb.Account) string {
	if account == nil {
		return ""
	}
	if value := NormalizeEmail(account.GetPrimaryMailboxEmail()); value != "" {
		return value
	}
	return CanonicalEmail(gptaccount.Email(account))
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

func EnrichMessage(message *mailboxv1.EmailInboxMessage) *mailboxv1.EmailInboxMessage {
	if message == nil {
		return nil
	}
	code, evidence := ExtractOTP(message)
	if code == "" {
		message.Signals = nil
		message.PrimarySignal = nil
		return message
	}
	signal := &mailboxv1.EmailSignal{
		Kind:            mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_OTP,
		SecretRef:       existingOTPSecretRef(message),
		Label:           "verification_code",
		Profile:         "gpt",
		Parser:          "gpt-account-mail",
		Confidence:      70,
		EvidencePreview: evidence,
	}
	message.Signals = []*mailboxv1.EmailSignal{signal}
	message.PrimarySignal = signal
	return message
}

func OTPCode(message *mailboxv1.EmailInboxMessage) string {
	if message == nil {
		return ""
	}
	code, _ := ExtractOTP(message)
	return code
}

func existingOTPSecretRef(message *mailboxv1.EmailInboxMessage) *commonv1.SecretRef {
	if message == nil {
		return nil
	}
	if signal := message.GetPrimarySignal(); signal.GetKind() == mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_OTP && signal.GetSecretRef().GetSecretId() != "" {
		return signal.GetSecretRef()
	}
	for _, signal := range message.GetSignals() {
		if signal.GetKind() == mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_OTP && signal.GetSecretRef().GetSecretId() != "" {
			return signal.GetSecretRef()
		}
	}
	return nil
}

func ExtractOTP(message *mailboxv1.EmailInboxMessage) (string, string) {
	if message == nil {
		return "", ""
	}
	text := strings.Join([]string{
		message.GetSubject(),
		message.GetFromAddress(),
		message.GetBodyPreview(),
	}, "\n")
	if match := verificationCodePattern.FindStringSubmatch(text); len(match) >= 2 {
		return channelotpwait.NormalizeCode(match[1]), strings.TrimSpace(match[0])
	}
	if match := standaloneCodePattern.FindStringSubmatch(text); len(match) >= 3 {
		return channelotpwait.NormalizeCode(match[2]), strings.TrimSpace(match[0])
	}
	return "", ""
}

func messageContainsPhrase(message Message, phrase string) bool {
	text := normalizeMailText(strings.Join([]string{
		message.Subject,
		message.BodyPreview,
		message.BodyText,
		message.HTMLBody,
	}, "\n"))
	return strings.Contains(text, normalizeMailText(phrase))
}

func messageSubjectEquals(message Message, subject string) bool {
	return normalizeMailText(message.Subject) == normalizeMailText(subject)
}

func normalizeMailText(value string) string {
	value = strings.NewReplacer("’", "'", "‘", "'", "`", "'", " ", " ").Replace(value)
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}
