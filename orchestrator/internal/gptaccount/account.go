package gptaccount

import (
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/accountmodel"
	"github.com/byte-v-forge/common-lib/emailx"
	accountv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/account/v1"

	"orchestrator/pb"
)

const stateErrorCode = "GPT_ACCOUNT_STATE_ERROR"
const CredentialKindCodexPhone = "codex_phone"

var Descriptor = accountmodel.Descriptor{SourceService: "gpt-account", AccountType: "gpt", ProviderKey: "openai"}

func New(accountID string, email string, status string) *pb.Account {
	options := []accountmodel.AccountOption{}
	if email = emailx.Normalize(email); email != "" {
		options = append(options, accountmodel.WithEmailIdentity(email, email))
	}
	if status = strings.TrimSpace(status); status != "" {
		options = append(options, accountmodel.WithStatus(accountmodel.StatusWithError(status, "", stateErrorCode, "", false)))
	}
	return &pb.Account{Account: Descriptor.Account(strings.TrimSpace(accountID), options...)}
}

func Patch(accountID string) *pb.Account {
	return &pb.Account{Account: Descriptor.Account(strings.TrimSpace(accountID))}
}

func ID(account *pb.Account) string {
	if account == nil {
		return ""
	}
	return accountmodel.AccountID(account.GetAccount())
}

func Email(account *pb.Account) string {
	if account == nil {
		return ""
	}
	return accountmodel.SubjectEmail(account.GetAccount())
}

func Status(account *pb.Account) string {
	if account == nil {
		return ""
	}
	return accountmodel.StatusValue(account.GetAccount())
}

func ErrorMessage(account *pb.Account) string {
	if account == nil {
		return ""
	}
	return accountmodel.ErrorMessage(account.GetAccount())
}

func SetStatus(account *pb.Account, status string, errorMessage string) {
	record := ensureRecord(account)
	if record == nil {
		return
	}
	record.Status = accountmodel.StatusWithError(status, "", stateErrorCode, errorMessage, false)
}

func SetCredential(account *pb.Account, kind string, present bool, status string, updatedAt time.Time) {
	record := ensureRecord(account)
	if record == nil {
		return
	}
	kind = strings.TrimSpace(kind)
	status = strings.TrimSpace(status)
	if kind == "" {
		return
	}
	accountmodel.SetCredentialState(record, kind, present, status, time.Time{}, updatedAt)
}

func CredentialUpdatedAtUnix(account *pb.Account, kind string) int64 {
	if account == nil {
		return 0
	}
	return accountmodel.CredentialUpdatedAtUnix(account.GetAccount(), kind)
}

func ensureRecord(account *pb.Account) *accountv1.Account {
	if account == nil {
		return nil
	}
	if account.Account == nil {
		account.Account = Descriptor.Account("")
	}
	return account.Account
}
