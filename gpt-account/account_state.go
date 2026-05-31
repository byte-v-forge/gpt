package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/accountmodel"
	"github.com/byte-v-forge/common-lib/accountstate"
	"github.com/byte-v-forge/common-lib/redisx"
	"github.com/byte-v-forge/gpt/pkg/gptplugin"

	"gpt_account/pb"
)

const (
	accountStateDefaultPrefix = "byte-v-forge:gpt:account-state"
	accountStateDefaultTTL    = 0

	stateFieldStatus              = "status"
	stateFieldErrorMessage        = "error_message"
	stateFieldChargeRef           = "charge_ref"
	stateFieldPlusTrialEligible   = "plus_trial_eligible"
	stateFieldPlusActive          = "plus_active"
	stateFieldTier                = "tier"
	stateFieldActivationChannel   = "activation_channel"
	stateFieldMailboxPresent      = "credential.mailbox.present"
	stateFieldMailboxStatus       = "credential.mailbox.status"
	stateFieldMailboxUpdatedAt    = "credential.mailbox.updated_at_unix"
	stateFieldCodexPhonePresent   = "credential.codex_phone.present"
	stateFieldCodexPhoneStatus    = "credential.codex_phone.status"
	stateFieldCodexPhoneUpdatedAt = "credential.codex_phone.updated_at_unix"
	stateFieldUpdatedAtUnix       = accountstate.DefaultUpdatedAtField
)

var accountStateFields = []string{
	stateFieldStatus,
	stateFieldErrorMessage,
	stateFieldChargeRef,
	stateFieldPlusTrialEligible,
	stateFieldPlusActive,
	stateFieldTier,
	stateFieldActivationChannel,
	stateFieldMailboxPresent,
	stateFieldMailboxStatus,
	stateFieldMailboxUpdatedAt,
	stateFieldCodexPhonePresent,
	stateFieldCodexPhoneStatus,
	stateFieldCodexPhoneUpdatedAt,
	stateFieldUpdatedAtUnix,
}

type accountStateStore struct {
	store *accountstate.AccountHashStore
}

func newAccountStateStore(ctx context.Context) (*accountStateStore, func() error, error) {
	rawURL := firstNonEmptyEnv("GPT_ACCOUNT_STATE_REDIS_URL", "GPT_RUNTIME_SECRET_REDIS_URL", "PLATFORM_REDIS_URL")
	client, err := redisx.NewRequiredClient(ctx, rawURL, "GPT account state redis url is required")
	if err != nil {
		return nil, nil, err
	}
	prefix := strings.TrimSpace(os.Getenv("GPT_ACCOUNT_STATE_KEY_PREFIX"))
	if prefix == "" {
		prefix = accountStateDefaultPrefix
	}
	store := accountstate.NewAccountHashStore(accountstate.AccountHashStoreConfig{Client: client, Prefix: prefix, TTL: accountStateDefaultTTL, Descriptor: gptAccountDescriptor})
	return &accountStateStore{store: store}, client.Close, nil
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func (s *accountStateStore) apply(ctx context.Context, account *pb.Account) error {
	accountID := gptAccountID(account)
	if s == nil || accountID == "" {
		return nil
	}
	values, err := s.store.Load(ctx, accountID, accountStateFields...)
	if err != nil {
		return err
	}
	setGptAccountStatus(account, stringDefault(values[stateFieldStatus], gptplugin.AccountStatusUnregistered), values[stateFieldErrorMessage])
	account.ChargeRef = values[stateFieldChargeRef]
	account.PlusTrialEligible = optionalBool(values, stateFieldPlusTrialEligible)
	account.PlusActive = optionalBool(values, stateFieldPlusActive)
	account.Tier = values[stateFieldTier]
	account.ActivationChannel = optionalString(values, stateFieldActivationChannel)
	if hasAccountStateValue(values, stateFieldMailboxPresent, stateFieldMailboxStatus, stateFieldMailboxUpdatedAt) {
		accountmodel.SetCredentialState(ensureGptAccountRecord(account), accountmodel.CredentialKindMailbox, boolValue(values[stateFieldMailboxPresent]), values[stateFieldMailboxStatus], time.Time{}, accountmodel.UnixTime(int64Value(values[stateFieldMailboxUpdatedAt])))
	}
	if hasAccountStateValue(values, stateFieldCodexPhonePresent, stateFieldCodexPhoneStatus, stateFieldCodexPhoneUpdatedAt) {
		accountmodel.SetCredentialState(ensureGptAccountRecord(account), credentialKindCodexPhone, boolValue(values[stateFieldCodexPhonePresent]), values[stateFieldCodexPhoneStatus], time.Time{}, accountmodel.UnixTime(int64Value(values[stateFieldCodexPhoneUpdatedAt])))
	}
	setGptAccountUpdatedAtUnix(account, int64Value(values[stateFieldUpdatedAtUnix]))
	return nil
}

func (s *accountStateStore) saveInitial(ctx context.Context, accountID string, account *pb.Account) error {
	values := accountStatePatchValues(account)
	if strings.TrimSpace(values[stateFieldStatus]) == "" {
		values[stateFieldStatus] = gptplugin.AccountStatusUnregistered
		values[stateFieldErrorMessage] = ""
	}
	return s.saveValues(ctx, accountID, values)
}

func (s *accountStateStore) savePatch(ctx context.Context, accountID string, account *pb.Account) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("account state store is not configured")
	}
	values := accountStatePatchValues(account)
	if len(values) == 0 {
		return nil
	}
	if err := s.store.PreserveMaxInt64(ctx, accountID, values, stateFieldMailboxUpdatedAt); err != nil {
		return err
	}
	return s.saveValues(ctx, accountID, values)
}

func (s *accountStateStore) saveValues(ctx context.Context, accountID string, values map[string]string) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("account state store is not configured")
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" || len(values) == 0 {
		return nil
	}
	return s.store.SavePatch(ctx, accountID, values)
}

func (s *accountStateStore) delete(ctx context.Context, accountID string) error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.Delete(ctx, accountID)
}

func hasAccountStateValue(values map[string]string, fields ...string) bool {
	for _, field := range fields {
		if _, ok := values[field]; ok {
			return true
		}
	}
	return false
}
