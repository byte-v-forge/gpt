package accountmail

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/emailx"
	"github.com/byte-v-forge/common-lib/eventbus"
	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"
	"github.com/byte-v-forge/common-lib/hotstream"
	"google.golang.org/grpc"

	"orchestrator/pb"
)

type AccountClient interface {
	ListAccounts(context.Context, *pb.ListAccountsRequest, ...grpc.CallOption) (*pb.ListAccountsResponse, error)
	UpdateAccount(context.Context, *pb.UpdateAccountRequest, ...grpc.CallOption) (*pb.UpdateAccountResponse, error)
}

type Projector struct {
	accounts AccountClient
	hot      hotstream.Publisher
}

func NewProjector(accounts AccountClient) *Projector {
	if accounts == nil {
		return nil
	}
	return &Projector{accounts: accounts}
}

func (p *Projector) WithHotStream(publisher hotstream.Publisher) *Projector {
	if p == nil {
		return nil
	}
	p.hot = publisher
	return p
}

func (p *Projector) ProjectMailboxEmail(ctx context.Context, event *mailboxv1.MailboxEmailReceivedEvent) error {
	if p == nil || p.accounts == nil || event == nil || event.GetMessage() == nil {
		return nil
	}
	message := EnrichMessage(event.GetMessage())
	if !IsOpenAIMessage(message) {
		return nil
	}
	accounts, err := p.accountsForMessage(ctx, message)
	if err != nil || len(accounts) == 0 {
		return err
	}
	for _, account := range accounts {
		if err := p.projectAccount(ctx, account, message); err != nil {
			return err
		}
		p.publishMailboxHotStream(ctx, account, message)
	}
	return nil
}

func (p *Projector) accountsForMessage(ctx context.Context, message *mailboxv1.EmailInboxMessage) ([]*pb.Account, error) {
	seen := map[string]struct{}{}
	out := []*pb.Account{}
	for _, email := range messageEmails(message) {
		resp, err := p.accounts.ListAccounts(ctx, &pb.ListAccountsRequest{Email: email, Limit: 10})
		if err != nil {
			return nil, err
		}
		for _, account := range resp.GetAccounts() {
			if account == nil || strings.TrimSpace(account.GetAccountId()) == "" {
				continue
			}
			if _, ok := seen[account.GetAccountId()]; ok {
				continue
			}
			seen[account.GetAccountId()] = struct{}{}
			out = append(out, account)
		}
	}
	return out, nil
}

func (p *Projector) projectAccount(ctx context.Context, account *pb.Account, message *mailboxv1.EmailInboxMessage) error {
	state := DetectState(account, MessagesFromPublic([]*mailboxv1.EmailInboxMessage{message}))
	update := &pb.Account{AccountId: account.GetAccountId()}
	needsUpdate := false
	if message.GetReceivedAtUnix() > 0 && message.GetReceivedAtUnix() >= account.GetMailboxLastMessageAtUnix() {
		update.MailboxLastFetchedAtUnix = message.GetReceivedAtUnix()
		update.MailboxLastMessageAtUnix = message.GetReceivedAtUnix()
		needsUpdate = true
	}
	if state.Status != "" {
		update.Status = state.Status
		needsUpdate = true
	}
	if state.ErrorMessage != "" {
		update.ErrorMessage = state.ErrorMessage
	}
	if state.Status == StatusDeactivated {
		active := false
		update.PlusActive = &active
	}
	if state.PlusActive {
		active := true
		update.PlusActive = &active
		if state.Tier != "" {
			update.Tier = state.Tier
		}
	}
	if !needsUpdate {
		return nil
	}
	_, err := p.accounts.UpdateAccount(ctx, &pb.UpdateAccountRequest{Account: update})
	return err
}

const (
	MailboxHotStreamEventUpdated = "gpt.account_mailbox.updated"
	MailboxHotStreamResource     = "gpt.account_mailbox"
	mailboxHotStreamSource       = "gpt-mailbox-projector"
)

func (p *Projector) publishMailboxHotStream(ctx context.Context, account *pb.Account, message *mailboxv1.EmailInboxMessage) {
	if p == nil || p.hot == nil || account == nil || message == nil {
		return
	}
	accountID := strings.TrimSpace(account.GetAccountId())
	if accountID == "" {
		return
	}
	receivedAt := message.GetReceivedAtUnix()
	if receivedAt <= 0 {
		receivedAt = time.Now().Unix()
	}
	event := hotstream.NewEvent(hotstream.EventConfig{
		EventID:       eventbus.StableEventID("gpt-account-mailbox-", accountID, strings.TrimSpace(message.GetId()), fmt.Sprintf("%d", receivedAt)),
		EventType:     MailboxHotStreamEventUpdated,
		SourceService: mailboxHotStreamSource,
		ResourceType:  MailboxHotStreamResource,
		ResourceID:    accountID,
		OccurredAt:    time.Unix(receivedAt, 0),
		CorrelationID: accountID,
		Attributes: map[string]string{
			"account_id":                accountID,
			"email":                     NormalizeEmail(account.GetEmail()),
			"mailbox_email":             NormalizeEmail(message.GetMailboxEmail()),
			"received_at_unix":          fmt.Sprintf("%d", receivedAt),
			"has_otp":                   fmt.Sprintf("%t", OTPCode(message) != ""),
			"mailbox_last_message_unix": fmt.Sprintf("%d", receivedAt),
		},
	})
	if err := p.hot.Publish(context.WithoutCancel(ctx), event); err != nil {
		log.Printf("[orchestrator] publish account mailbox hotstream failed account=%s: %v", accountID, err)
	}
}

func messageEmails(message *mailboxv1.EmailInboxMessage) []string {
	if message == nil {
		return nil
	}
	values := []string{message.GetMailboxEmail(), message.GetSourceMailboxEmail()}
	values = append(values, message.GetRecipients()...)
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		for _, candidate := range []string{value, emailx.CanonicalPlusAlias(value)} {
			candidate = NormalizeEmail(candidate)
			if candidate == "" {
				continue
			}
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			out = append(out, candidate)
		}
	}
	return out
}
