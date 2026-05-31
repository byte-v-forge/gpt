package main

import (
	"strings"

	"github.com/byte-v-forge/common-lib/accountmodel"
	"github.com/byte-v-forge/common-lib/emailx"
	accountv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/account/v1"
	"github.com/byte-v-forge/gpt/pkg/gptplugin"

	"gpt_account/pb"
)

const gptAccountStateErrorCode = "GPT_ACCOUNT_STATE_ERROR"

func newGptAccountRecord(accountID string, email string, createdAtUnix int64, updatedAtUnix int64) *accountv1.Account {
	email = emailx.Normalize(email)
	return gptAccountDescriptor.Account(
		strings.TrimSpace(accountID),
		accountmodel.WithEmailIdentity(email, email),
		accountmodel.WithCreatedTimestamp(accountmodel.UnixTimestamp(createdAtUnix)),
		accountmodel.WithUpdatedTimestamp(accountmodel.UnixTimestamp(updatedAtUnix)),
	)
}

func ensureGptAccountRecord(account *pb.Account) *accountv1.Account {
	if account == nil {
		return nil
	}
	if account.Account == nil {
		account.Account = gptAccountDescriptor.Account("")
	}
	return account.Account
}

func gptAccountID(account *pb.Account) string {
	if account == nil {
		return ""
	}
	return accountmodel.AccountID(account.GetAccount())
}

func gptAccountEmail(account *pb.Account) string {
	if account == nil {
		return ""
	}
	return accountmodel.SubjectEmail(account.GetAccount())
}

func gptAccountStatusValue(account *pb.Account) string {
	if account == nil {
		return ""
	}
	return accountmodel.StatusValue(account.GetAccount())
}

func gptAccountErrorMessage(account *pb.Account) string {
	if account == nil {
		return ""
	}
	return accountmodel.ErrorMessage(account.GetAccount())
}

func gptAccountCreatedAtUnix(account *pb.Account) int64 {
	if account == nil {
		return 0
	}
	return accountmodel.CreatedAtUnix(account.GetAccount())
}

func gptAccountUpdatedAtUnix(account *pb.Account) int64 {
	if account == nil {
		return 0
	}
	return accountmodel.UpdatedAtUnix(account.GetAccount())
}

func setGptAccountStatus(account *pb.Account, value string, message string) {
	record := ensureGptAccountRecord(account)
	if record == nil {
		return
	}
	value = strings.TrimSpace(value)
	if value == "" {
		value = gptplugin.AccountStatusUnregistered
	}
	record.Status = accountmodel.StatusWithError(value, "", gptAccountStateErrorCode, message, false)
}

func setGptAccountUpdatedAtUnix(account *pb.Account, value int64) {
	record := ensureGptAccountRecord(account)
	if record == nil || value <= 0 {
		return
	}
	accountmodel.SetUpdatedAtUnixMax(record, value)
}
