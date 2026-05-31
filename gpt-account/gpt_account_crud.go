package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/accountcrud"
	"github.com/byte-v-forge/common-lib/accountmodel"
	"github.com/byte-v-forge/common-lib/emailx"
	accountv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/account/v1"
	"github.com/byte-v-forge/common-lib/pagex"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	"gpt_account/db"
	"gpt_account/pb"
)

const maxAccountListScanBatches = 8

type gptAccountCRUDRecord struct {
	Account    *pb.Account
	Credential *pb.AccountCredential
	Create     bool
}

type gptAccountStore struct {
	server       *gptAccountServer
	emailFilter  string
	statusFilter string
}

func (s *gptAccountServer) gptAccounts(emailFilter string, statusFilter string) *accountcrud.Manager[gptAccountCRUDRecord] {
	store := gptAccountStore{server: s, emailFilter: emailx.Normalize(emailFilter), statusFilter: strings.TrimSpace(statusFilter)}
	return accountcrud.New[gptAccountCRUDRecord](accountcrud.Config[gptAccountCRUDRecord]{
		Store: accountcrud.StoreFuncs[gptAccountCRUDRecord]{
			Name:       "gpt account store",
			ListFunc:   store.list,
			GetFunc:    store.get,
			UpsertFunc: store.upsert,
			DeleteFunc: store.delete,
		},
		Descriptor: gptAccountDescriptor,
		AccountOf: func(record gptAccountCRUDRecord) *accountv1.Account {
			if record.Create {
				return nil
			}
			return gptAccountProjection(record.Account)
		},
		Publishers: s.gptAccountPublishers(),
		IDField:    "account_id",
	})
}

func (s *gptAccountServer) createGPTAccount(ctx context.Context, account *pb.Account, credential *pb.AccountCredential) (*pb.Account, error) {
	record, err := s.gptAccounts("", "").Upsert(ctx, gptAccountCRUDRecord{Account: account, Credential: credential, Create: true})
	return record.Account, err
}

func (s *gptAccountServer) updateGPTAccount(ctx context.Context, account *pb.Account, credential *pb.AccountCredential) (*pb.Account, error) {
	record, err := s.gptAccounts("", "").Update(ctx, gptAccountCRUDRecord{Account: account, Credential: credential})
	return record.Account, err
}

func (s *gptAccountServer) getGPTAccount(ctx context.Context, accountID string) (*pb.Account, error) {
	record, found, err := s.gptAccounts("", "").Get(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, status.Error(codes.NotFound, "account not found")
	}
	return record.Account, nil
}

func (s *gptAccountServer) deleteGPTAccount(ctx context.Context, accountID string) error {
	_, err := s.gptAccounts("", "").Delete(ctx, accountID)
	return err
}

func (s *gptAccountServer) listGPTAccounts(ctx context.Context, req *pb.ListAccountsRequest) (*pb.ListAccountsResponse, error) {
	page, err := s.gptAccounts(req.GetEmail(), req.GetStatus()).List(ctx, accountcrud.ListRequest{Cursor: req.GetCursor(), Limit: int(req.GetLimit())})
	if err != nil {
		return nil, err
	}
	resp := &pb.ListAccountsResponse{Accounts: make([]*pb.Account, 0, len(page.Records)), NextCursor: page.NextCursor}
	for _, record := range page.Records {
		resp.Accounts = append(resp.Accounts, record.Account)
	}
	return resp, nil
}

func (s gptAccountStore) list(ctx context.Context, req accountcrud.ListRequest) (accountcrud.Page[gptAccountCRUDRecord], error) {
	limit := accountmodel.NormalizePageLimit(req.Limit)
	cursor, err := pagex.DecodeKeysetCursor(req.Cursor)
	if err != nil {
		return accountcrud.Page[gptAccountCRUDRecord]{}, status.Error(codes.InvalidArgument, err.Error())
	}
	records := make([]gptAccountCRUDRecord, 0, limit)
	nextCursor := cursor
	exhausted := false
	for scans := 0; len(records) < limit && scans < maxAccountListScanBatches; scans++ {
		page, err := s.accountPageRows(ctx, nextCursor, accountListBatchLimit(limit-len(records), s.statusFilter != ""))
		if err != nil {
			return accountcrud.Page[gptAccountCRUDRecord]{}, err
		}
		rows := page.Items
		if len(rows) == 0 {
			exhausted = true
			break
		}
		for i := range rows {
			nextCursor = dbAccountPageCursor(&rows[i])
			account, err := s.server.accountToProto(ctx, &rows[i])
			if err != nil {
				return accountcrud.Page[gptAccountCRUDRecord]{}, err
			}
			if s.statusFilter != "" && gptAccountStatusValue(account) != s.statusFilter {
				continue
			}
			records = append(records, gptAccountCRUDRecord{Account: account})
			if len(records) >= limit {
				break
			}
		}
		if !page.HasMore {
			exhausted = true
			break
		}
	}
	next := ""
	if !exhausted && pagex.HasKeysetCursor(nextCursor) {
		next = pagex.EncodeKeysetCursorValue(nextCursor)
	}
	return accountcrud.Page[gptAccountCRUDRecord]{Records: records, NextCursor: next}, nil
}

func (s gptAccountStore) get(ctx context.Context, accountID string) (gptAccountCRUDRecord, bool, error) {
	row, err := s.server.findAccount(ctx, accountID)
	if status.Code(err) == codes.NotFound {
		return gptAccountCRUDRecord{}, false, nil
	}
	if err != nil {
		return gptAccountCRUDRecord{}, false, err
	}
	account, err := s.server.accountToProto(ctx, row)
	if err != nil {
		return gptAccountCRUDRecord{}, false, err
	}
	return gptAccountCRUDRecord{Account: account}, true, nil
}

func (s gptAccountStore) upsert(ctx context.Context, record gptAccountCRUDRecord) (gptAccountCRUDRecord, error) {
	if record.Create {
		return s.create(ctx, record)
	}
	return s.update(ctx, record)
}

func (s gptAccountStore) delete(ctx context.Context, accountID string) (gptAccountCRUDRecord, bool, error) {
	accountID, err := normalizeAccountID(accountID)
	if err != nil {
		return gptAccountCRUDRecord{}, false, err
	}
	result := s.server.db.WithContext(ctx).Delete(&db.Account{}, "id = ?", accountID)
	if result.Error != nil {
		return gptAccountCRUDRecord{}, false, result.Error
	}
	if result.RowsAffected == 0 {
		return gptAccountCRUDRecord{}, false, nil
	}
	if err := s.server.state.delete(ctx, accountID); err != nil {
		return gptAccountCRUDRecord{}, false, err
	}
	return gptAccountCRUDRecord{}, true, nil
}

func (s gptAccountStore) create(ctx context.Context, record gptAccountCRUDRecord) (gptAccountCRUDRecord, error) {
	row, err := s.server.buildAccount(record.Account, record.Credential)
	if err != nil {
		return gptAccountCRUDRecord{}, err
	}
	if row.Email == "" {
		return gptAccountCRUDRecord{}, status.Error(codes.InvalidArgument, "email is required")
	}
	if err := s.server.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		return assignAccountEmailAllocation(tx, row.ID, row.Email)
	}); err != nil {
		return gptAccountCRUDRecord{}, err
	}
	if err := s.server.state.saveInitial(ctx, row.ID, record.Account); err != nil {
		return gptAccountCRUDRecord{}, err
	}
	account, err := s.server.accountToProto(ctx, row)
	if err != nil {
		return gptAccountCRUDRecord{}, err
	}
	log.Printf("Created account id=%s email=%s", row.ID, emailx.Redact(row.Email))
	return gptAccountCRUDRecord{Account: account}, nil
}

func (s gptAccountStore) update(ctx context.Context, record gptAccountCRUDRecord) (gptAccountCRUDRecord, error) {
	accountID, err := normalizeAccountID(gptAccountID(record.Account))
	if err != nil {
		return gptAccountCRUDRecord{}, err
	}
	if _, err := s.server.findAccount(ctx, accountID); err != nil {
		return gptAccountCRUDRecord{}, err
	}
	updates := updateMap(record.Account, record.Credential)
	if len(updates) > 0 {
		if err := s.server.db.WithContext(ctx).Model(&db.Account{}).Where("id = ?", accountID).Updates(updates).Error; err != nil {
			return gptAccountCRUDRecord{}, err
		}
	}
	if err := s.server.state.savePatch(ctx, accountID, record.Account); err != nil {
		return gptAccountCRUDRecord{}, err
	}
	row, err := s.server.findAccount(ctx, accountID)
	if err != nil {
		return gptAccountCRUDRecord{}, err
	}
	account, err := s.server.accountToProto(ctx, row)
	if err != nil {
		return gptAccountCRUDRecord{}, err
	}
	log.Printf("Updated account id=%s status=%s", row.ID, gptAccountStatusValue(account))
	return gptAccountCRUDRecord{Account: account}, nil
}

func (s gptAccountStore) accountPageRows(ctx context.Context, cursor pagex.KeysetCursor, limit int) (pagex.KeysetPage[db.Account], error) {
	limit = accountmodel.NormalizePageLimit(limit)
	query := s.server.db.WithContext(ctx).Order("updated_at DESC, id DESC").Limit(pagex.KeysetLookaheadLimit(limit))
	if s.emailFilter != "" {
		query = query.Where("email = ?", s.emailFilter)
	}
	if pagex.HasKeysetCursor(cursor) {
		query = query.Where("(updated_at, id) < (?, ?)", cursor.UpdatedAt.Unix(), cursor.ID)
	}
	var rows []db.Account
	if err := query.Find(&rows).Error; err != nil {
		return pagex.KeysetPage[db.Account]{}, err
	}
	return pagex.NewKeysetPage(rows, limit, func(row db.Account) pagex.KeysetCursor {
		return dbAccountPageCursor(&row)
	}), nil
}

func accountListBatchLimit(remaining int, filtered bool) int {
	if remaining <= 0 {
		return accountmodel.DefaultPageLimit
	}
	if !filtered {
		return remaining
	}
	if remaining < accountmodel.DefaultPageLimit {
		return accountmodel.DefaultPageLimit
	}
	return remaining
}

func dbAccountPageCursor(account *db.Account) pagex.KeysetCursor {
	if account == nil || account.UpdatedAt <= 0 || account.ID == "" {
		return pagex.KeysetCursor{}
	}
	return pagex.KeysetCursor{UpdatedAt: time.Unix(account.UpdatedAt, 0).UTC(), ID: account.ID}
}

func (s *gptAccountServer) gptAccountPublishers() []accountcrud.ChangePublisher {
	if s == nil || s.accountStream == nil {
		return nil
	}
	return []accountcrud.ChangePublisher{
		accountcrud.BestEffortPublisher(s.accountStream, func(_ context.Context, _ accountv1.AccountChangeKind, account *accountv1.Account, err error) {
			if account != nil {
				log.Printf("publish account hotstream failed account=%s: %v", account.GetKey().GetAccountId(), err)
			}
		}),
	}
}
