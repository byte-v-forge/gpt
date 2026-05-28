package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/redisx"

	"gpt_account/pb"
)

const (
	accountStatusUnregistered = "UNREGISTERED"

	accountStateDefaultPrefix = "byte-v-forge:gpt:account-state"
	accountStateDefaultTTL    = 0

	stateFieldStatus               = "status"
	stateFieldErrorMessage         = "error_message"
	stateFieldChargeRef            = "charge_ref"
	stateFieldPlusTrialEligible    = "plus_trial_eligible"
	stateFieldPlusActive           = "plus_active"
	stateFieldTier                 = "tier"
	stateFieldActivationChannel    = "activation_channel"
	stateFieldMailboxFetchedAtUnix = "mailbox_last_fetched_at_unix"
	stateFieldMailboxMessageAtUnix = "mailbox_last_message_at_unix"
	stateFieldCodexPhoneConfirmed  = "codex_phone_confirmed"
	stateFieldCodexPhoneLabel      = "codex_phone_label"
	stateFieldCodexPhoneUpdatedAt  = "codex_phone_updated_at_unix"
	stateFieldCodexPhoneStatus     = "codex_phone_status"
	stateFieldUpdatedAtUnix        = "updated_at_unix"
)

var accountStateFields = []string{
	stateFieldStatus,
	stateFieldErrorMessage,
	stateFieldChargeRef,
	stateFieldPlusTrialEligible,
	stateFieldPlusActive,
	stateFieldTier,
	stateFieldActivationChannel,
	stateFieldMailboxFetchedAtUnix,
	stateFieldMailboxMessageAtUnix,
	stateFieldCodexPhoneConfirmed,
	stateFieldCodexPhoneLabel,
	stateFieldCodexPhoneUpdatedAt,
	stateFieldCodexPhoneStatus,
	stateFieldUpdatedAtUnix,
}

type accountStateStore struct {
	store *redisx.StringStore
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
	store := redisx.NewStringStore(client, prefix, accountStateDefaultTTL)
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
	if s == nil || account == nil || strings.TrimSpace(account.GetAccountId()) == "" {
		return nil
	}
	values, err := s.store.HashLoadMany(ctx, stateKey(account.GetAccountId()), accountStateFields...)
	if err != nil {
		return err
	}
	account.Status = stringDefault(values[stateFieldStatus], accountStatusUnregistered)
	account.ErrorMessage = values[stateFieldErrorMessage]
	account.ChargeRef = values[stateFieldChargeRef]
	account.PlusTrialEligible = optionalBool(values, stateFieldPlusTrialEligible)
	account.PlusActive = optionalBool(values, stateFieldPlusActive)
	account.Tier = values[stateFieldTier]
	account.ActivationChannel = optionalString(values, stateFieldActivationChannel)
	account.MailboxLastFetchedAtUnix = int64Value(values[stateFieldMailboxFetchedAtUnix])
	account.MailboxLastMessageAtUnix = int64Value(values[stateFieldMailboxMessageAtUnix])
	account.CodexPhoneConfirmed = optionalBool(values, stateFieldCodexPhoneConfirmed)
	account.CodexPhoneLabel = values[stateFieldCodexPhoneLabel]
	account.CodexPhoneUpdatedAtUnix = int64Value(values[stateFieldCodexPhoneUpdatedAt])
	account.CodexPhoneStatus = values[stateFieldCodexPhoneStatus]
	return nil
}

func (s *accountStateStore) saveInitial(ctx context.Context, accountID string, account *pb.Account) error {
	values := accountStatePatchValues(account)
	if strings.TrimSpace(values[stateFieldStatus]) == "" {
		values[stateFieldStatus] = accountStatusUnregistered
		values[stateFieldErrorMessage] = ""
	}
	return s.saveValues(ctx, accountID, values)
}

func (s *accountStateStore) savePatch(ctx context.Context, accountID string, account *pb.Account) error {
	values := accountStatePatchValues(account)
	if len(values) == 0 {
		return nil
	}
	if messageAt := int64Value(values[stateFieldMailboxMessageAtUnix]); messageAt > 0 {
		existing, err := s.store.HashLoadMany(ctx, stateKey(accountID), stateFieldMailboxMessageAtUnix)
		if err != nil {
			return err
		}
		if current := int64Value(existing[stateFieldMailboxMessageAtUnix]); current > messageAt {
			values[stateFieldMailboxMessageAtUnix] = strconv.FormatInt(current, 10)
		}
	}
	return s.saveValues(ctx, accountID, values)
}

func (s *accountStateStore) saveValues(ctx context.Context, accountID string, values map[string]string) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("account state store is not configured")
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil
	}
	if len(values) == 0 {
		return nil
	}
	values[stateFieldUpdatedAtUnix] = strconv.FormatInt(time.Now().Unix(), 10)
	return s.store.HashSaveTTL(ctx, stateKey(accountID), values, accountStateDefaultTTL)
}

func (s *accountStateStore) delete(ctx context.Context, accountID string) error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.Delete(ctx, stateKey(accountID))
}

func accountStatePatchValues(account *pb.Account) map[string]string {
	values := map[string]string{}
	if account == nil {
		return values
	}
	if value := strings.TrimSpace(account.GetStatus()); value != "" {
		values[stateFieldStatus] = value
		values[stateFieldErrorMessage] = account.GetErrorMessage()
	} else if account.GetErrorMessage() != "" {
		values[stateFieldErrorMessage] = account.GetErrorMessage()
	}
	if value := strings.TrimSpace(account.GetChargeRef()); value != "" {
		values[stateFieldChargeRef] = value
	}
	if account.PlusTrialEligible != nil {
		values[stateFieldPlusTrialEligible] = strconv.FormatBool(account.GetPlusTrialEligible())
	}
	if account.PlusActive != nil {
		values[stateFieldPlusActive] = strconv.FormatBool(account.GetPlusActive())
	}
	if value := normalizeTier(account.GetTier()); value != "" {
		values[stateFieldTier] = value
	}
	if account.ActivationChannel != nil {
		values[stateFieldActivationChannel] = strings.TrimSpace(account.GetActivationChannel())
	}
	if value := account.GetMailboxLastFetchedAtUnix(); value > 0 {
		values[stateFieldMailboxFetchedAtUnix] = strconv.FormatInt(value, 10)
	}
	if value := account.GetMailboxLastMessageAtUnix(); value > 0 {
		values[stateFieldMailboxMessageAtUnix] = strconv.FormatInt(value, 10)
	}
	phoneStatus := normalizeCodexPhoneStatus(account.GetCodexPhoneStatus())
	if account.CodexPhoneConfirmed != nil {
		updatedAt := account.GetCodexPhoneUpdatedAtUnix()
		if updatedAt <= 0 {
			updatedAt = time.Now().Unix()
		}
		values[stateFieldCodexPhoneConfirmed] = strconv.FormatBool(account.GetCodexPhoneConfirmed())
		values[stateFieldCodexPhoneLabel] = strings.TrimSpace(account.GetCodexPhoneLabel())
		values[stateFieldCodexPhoneUpdatedAt] = strconv.FormatInt(updatedAt, 10)
		if phoneStatus == "" && account.GetCodexPhoneConfirmed() {
			phoneStatus = codexPhoneStatusConfirmed
		}
	}
	if phoneStatus != "" {
		updatedAt := account.GetCodexPhoneUpdatedAtUnix()
		if updatedAt <= 0 {
			updatedAt = time.Now().Unix()
		}
		values[stateFieldCodexPhoneStatus] = phoneStatus
		values[stateFieldCodexPhoneUpdatedAt] = strconv.FormatInt(updatedAt, 10)
		if phoneStatus == codexPhoneStatusConfirmed && account.CodexPhoneConfirmed == nil {
			values[stateFieldCodexPhoneConfirmed] = "true"
		}
		if phoneStatus == codexPhoneStatusOAuthNeedPhone && account.CodexPhoneConfirmed == nil {
			values[stateFieldCodexPhoneConfirmed] = "false"
		}
		if label := strings.TrimSpace(account.GetCodexPhoneLabel()); label != "" && account.CodexPhoneConfirmed == nil {
			values[stateFieldCodexPhoneLabel] = label
		}
	}
	return values
}

func stateKey(accountID string) string {
	return "account:" + strings.TrimSpace(accountID)
}

func optionalBool(values map[string]string, field string) *bool {
	raw, ok := values[field]
	if !ok {
		return nil
	}
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return nil
	}
	return &value
}

func optionalString(values map[string]string, field string) *string {
	raw, ok := values[field]
	if !ok {
		return nil
	}
	value := strings.TrimSpace(raw)
	return &value
}

func int64Value(value string) int64 {
	out, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return out
}

func stringDefault(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
