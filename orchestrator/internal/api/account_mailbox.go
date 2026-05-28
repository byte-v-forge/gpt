package api

import (
	"context"
	"fmt"
	"strings"

	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"

	"orchestrator/internal/accountmail"
	"orchestrator/pb"
)

func (s *Server) FetchAccountMailbox(ctx context.Context, req *pb.FetchAccountMailboxRequest) (*pb.FetchAccountMailboxResponse, error) {
	accountID := strings.TrimSpace(req.GetAccountId())
	if accountID == "" {
		return &pb.FetchAccountMailboxResponse{ErrorMessage: "account_id is required"}, nil
	}
	accountResp, err := s.accountClient.GetAccount(ctx, &pb.GetAccountRequest{AccountId: accountID})
	if err != nil {
		return &pb.FetchAccountMailboxResponse{ErrorMessage: err.Error()}, nil
	}
	account := accountResp.GetAccount()
	if account == nil {
		return &pb.FetchAccountMailboxResponse{ErrorMessage: "account not found"}, nil
	}
	primaryMailbox := accountmail.PrimaryMailbox(account)
	if primaryMailbox == "" {
		return &pb.FetchAccountMailboxResponse{Account: account, ErrorMessage: "account primary mailbox is empty"}, nil
	}
	account, err = s.ensureAccountPrimaryMailbox(ctx, account, primaryMailbox)
	if err != nil {
		return &pb.FetchAccountMailboxResponse{Account: account, ErrorMessage: err.Error()}, nil
	}
	inbox := otpInbox(primaryMailbox, nil)
	if s.otpProjection == nil {
		return &pb.FetchAccountMailboxResponse{Account: account, Inbox: inbox, ErrorMessage: "otp cache is not configured"}, nil
	}
	message, _, found, err := s.otpProjection.LatestMailboxSignal(ctx, accountOTPEmail(account, primaryMailbox), mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_OTP, 0)
	if err != nil {
		return &pb.FetchAccountMailboxResponse{Account: account, ErrorMessage: fmt.Sprintf("read GPT mailbox projection: %v", err)}, nil
	}
	if found && message != nil {
		inbox = otpInbox(primaryMailbox, []*mailboxv1.EmailInboxMessage{message})
	}
	return &pb.FetchAccountMailboxResponse{Account: account, Inbox: inbox}, nil
}

func (s *Server) ensureAccountPrimaryMailbox(ctx context.Context, account *pb.Account, primaryMailbox string) (*pb.Account, error) {
	if accountmail.NormalizeEmail(account.GetPrimaryMailboxEmail()) == accountmail.NormalizeEmail(primaryMailbox) {
		return account, nil
	}
	resp, err := s.accountClient.UpdateAccount(ctx, &pb.UpdateAccountRequest{Account: &pb.Account{
		AccountId:           account.GetAccountId(),
		PrimaryMailboxEmail: primaryMailbox,
	}})
	if err != nil {
		return account, fmt.Errorf("update account primary mailbox: %w", err)
	}
	if resp.GetAccount() == nil {
		return account, fmt.Errorf("gpt-account returned empty account")
	}
	return resp.GetAccount(), nil
}

func otpInbox(email string, messages []*mailboxv1.EmailInboxMessage) *mailboxv1.FetchMailboxInboxesResponse {
	return &mailboxv1.FetchMailboxInboxesResponse{
		Results: []*mailboxv1.FetchMailboxInboxResult{{
			Mailbox:  projectedMailbox(email),
			Messages: messages,
		}},
		MailboxCount: 1,
		FetchedCount: 1,
		MessageCount: int32(len(messages)),
	}
}

func projectedMailbox(email string) *mailboxv1.EmailMailbox {
	email = accountmail.NormalizeEmail(email)
	_, domain, _ := strings.Cut(email, "@")
	return &mailboxv1.EmailMailbox{EmailAddress: email, Domain: domain}
}

func accountOTPEmail(account *pb.Account, primaryMailbox string) string {
	if value := accountmail.NormalizeEmail(account.GetEmail()); value != "" {
		return value
	}
	return primaryMailbox
}
