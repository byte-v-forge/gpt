package api

import (
	"context"
	"fmt"
	"strings"

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
	limit := req.GetLimitPerMailbox()
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	inbox, err := s.mailboxClient.FetchMailboxInboxes(ctx, &pb.FetchMailboxInboxesRequest{
		LimitPerMailbox:   limit,
		MaxMailboxes:      1,
		EmailAddress:      primaryMailbox,
		ReceivedAfterUnix: accountMailboxWatermark(account),
	})
	if err != nil {
		return &pb.FetchAccountMailboxResponse{Account: account, ErrorMessage: fmt.Sprintf("fetch account mailbox: %v", err)}, nil
	}
	updated, err := s.applyAccountMailboxState(ctx, account, primaryMailbox, inbox)
	if err != nil {
		return &pb.FetchAccountMailboxResponse{Account: account, Inbox: inbox, ErrorMessage: err.Error()}, nil
	}
	return &pb.FetchAccountMailboxResponse{Account: updated, Inbox: inbox}, nil
}

func (s *Server) SyncAccountMailboxes(ctx context.Context, req *pb.SyncAccountMailboxesRequest) (*pb.SyncAccountMailboxesResponse, error) {
	limit := mailboxFetchLimit(req.GetLimitPerMailbox())
	accountLimit := req.GetAccountLimit()
	if accountLimit <= 0 {
		accountLimit = 500
	}
	if accountLimit > 500 {
		accountLimit = 500
	}
	accountsResp, err := s.accountClient.ListAccounts(ctx, &pb.ListAccountsRequest{Limit: accountLimit})
	if err != nil {
		return &pb.SyncAccountMailboxesResponse{ErrorMessage: err.Error()}, nil
	}
	groups := map[string][]*pb.Account{}
	for _, account := range accountsResp.GetAccounts() {
		if account == nil {
			continue
		}
		if primaryMailbox := accountmail.PrimaryMailbox(account); primaryMailbox != "" {
			groups[primaryMailbox] = append(groups[primaryMailbox], account)
		}
	}
	resp := &pb.SyncAccountMailboxesResponse{
		AccountCount: int32(len(accountsResp.GetAccounts())),
		MailboxCount: int32(len(groups)),
		Success:      true,
	}
	for primaryMailbox, accounts := range groups {
		inbox, err := s.mailboxClient.FetchMailboxInboxes(ctx, &pb.FetchMailboxInboxesRequest{
			LimitPerMailbox:   limit,
			MaxMailboxes:      1,
			EmailAddress:      primaryMailbox,
			ReceivedAfterUnix: groupMailboxWatermark(accounts),
		})
		if err != nil {
			resp.Success = false
			resp.FailedCount += int32(len(accounts))
			resp.ErrorMessage = appendErrorMessage(resp.GetErrorMessage(), fmt.Sprintf("%s: %v", primaryMailbox, err))
			continue
		}
		resp.MessageCount += inbox.GetMessageCount()
		for _, account := range accounts {
			updated, err := s.applyAccountMailboxState(ctx, account, primaryMailbox, inbox)
			if err != nil {
				resp.Success = false
				resp.FailedCount++
				resp.ErrorMessage = appendErrorMessage(resp.GetErrorMessage(), fmt.Sprintf("%s: %v", account.GetAccountId(), err))
				continue
			}
			resp.SyncedCount++
			if updated.GetMailboxLastMessageAtUnix() > resp.GetMaxMessageAtUnix() {
				resp.MaxMessageAtUnix = updated.GetMailboxLastMessageAtUnix()
			}
		}
	}
	return resp, nil
}

func (s *Server) applyAccountMailboxState(ctx context.Context, account *pb.Account, primaryMailbox string, inbox *pb.FetchMailboxInboxesResponse) (*pb.Account, error) {
	messages := mailboxMessagesAfter(accountmail.InboxMessages(account, inbox), accountMailboxWatermark(account))
	latestMessageUnix := accountmail.LatestMessageUnix(account, messages)
	update := &pb.Account{
		AccountId:           account.GetAccountId(),
		PrimaryMailboxEmail: primaryMailbox,
	}
	needsUpdate := accountmail.NormalizeEmail(account.GetPrimaryMailboxEmail()) != accountmail.NormalizeEmail(primaryMailbox)
	if latestMessageUnix > 0 {
		update.MailboxLastFetchedAtUnix = latestMessageUnix
		update.MailboxLastMessageAtUnix = latestMessageUnix
		needsUpdate = true
	}
	state := accountmail.DetectState(account, messages)
	if state.Status != "" {
		update.Status = state.Status
		update.ErrorMessage = state.ErrorMessage
		needsUpdate = true
	}
	if otp := accountmail.LatestOTP(account, messages); otp.Code != "" {
		update.MailboxLatestOtp = otp.Code
		update.MailboxLatestOtpSubject = otp.Subject
		update.MailboxLatestOtpReceivedAtUnix = otp.ReceivedAtUnix
		needsUpdate = true
	}
	if state.PlusActive {
		value := true
		update.PlusActive = &value
		update.Tier = "plus"
		needsUpdate = true
	}
	if !needsUpdate {
		return account, nil
	}
	resp, err := s.accountClient.UpdateAccount(ctx, &pb.UpdateAccountRequest{Account: update})
	if err != nil {
		return nil, fmt.Errorf("update account mailbox state: %w", err)
	}
	if resp.GetAccount() == nil {
		return nil, fmt.Errorf("account-db returned empty account")
	}
	return resp.GetAccount(), nil
}

func mailboxFetchLimit(limit int32) int32 {
	if limit <= 0 {
		return 10
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func accountMailboxWatermark(account *pb.Account) int64 {
	return maxInt64(account.GetMailboxLastMessageAtUnix(), account.GetMailboxLatestOtpReceivedAtUnix())
}

func groupMailboxWatermark(accounts []*pb.Account) int64 {
	var watermark int64
	first := true
	for _, account := range accounts {
		value := accountMailboxWatermark(account)
		if first || value < watermark {
			watermark = value
			first = false
		}
	}
	return watermark
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func appendErrorMessage(existing, next string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return existing
	}
	if strings.TrimSpace(existing) == "" {
		return next
	}
	return existing + "; " + next
}

func mailboxMessagesAfter(messages []*pb.EmailInboxMessage, watermark int64) []*pb.EmailInboxMessage {
	if watermark <= 0 {
		return messages
	}
	out := make([]*pb.EmailInboxMessage, 0, len(messages))
	for _, message := range messages {
		if message.GetReceivedAtUnix() > watermark {
			out = append(out, message)
		}
	}
	return out
}
