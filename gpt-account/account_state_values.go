package main

import (
	"strconv"
	"strings"

	"github.com/byte-v-forge/common-lib/accountmodel"

	"gpt_account/pb"
)

func accountStatePatchValues(account *pb.Account) map[string]string {
	values := map[string]string{}
	if account == nil {
		return values
	}
	if value := gptAccountStatusValue(account); value != "" {
		values[stateFieldStatus] = value
		values[stateFieldErrorMessage] = gptAccountErrorMessage(account)
	} else if message := gptAccountErrorMessage(account); message != "" {
		values[stateFieldErrorMessage] = message
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
	if credential := accountmodel.CredentialState(account.GetAccount(), accountmodel.CredentialKindMailbox); credential != nil {
		values[stateFieldMailboxPresent] = strconv.FormatBool(credential.GetPresent())
		if status := strings.TrimSpace(credential.GetStatus()); status != "" {
			values[stateFieldMailboxStatus] = status
		}
		if updatedAt := accountmodel.TimestampUnix(credential.GetUpdatedAt()); updatedAt > 0 {
			values[stateFieldMailboxUpdatedAt] = strconv.FormatInt(updatedAt, 10)
		}
	}
	if credential := accountmodel.CredentialState(account.GetAccount(), credentialKindCodexPhone); credential != nil {
		values[stateFieldCodexPhonePresent] = strconv.FormatBool(credential.GetPresent())
		if status := normalizeCodexPhoneStatus(credential.GetStatus()); status != "" {
			values[stateFieldCodexPhoneStatus] = status
		}
		if updatedAt := accountmodel.TimestampUnix(credential.GetUpdatedAt()); updatedAt > 0 {
			values[stateFieldCodexPhoneUpdatedAt] = strconv.FormatInt(updatedAt, 10)
		}
	}
	return values
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

func boolValue(value string) bool {
	out, _ := strconv.ParseBool(strings.TrimSpace(value))
	return out
}

func stringDefault(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
