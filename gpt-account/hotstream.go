package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/byte-v-forge/common-lib/accountmodel"
	"github.com/byte-v-forge/common-lib/eventbus"
	accountv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/account/v1"
	"github.com/byte-v-forge/common-lib/hotstream"
	"github.com/byte-v-forge/common-lib/hotstreamnats"

	"gpt_account/pb"
)

var gptAccountDescriptor = accountmodel.Descriptor{SourceService: "gpt-account", AccountType: "gpt", ProviderKey: "openai"}

const (
	accountAllocationResource          = "gpt.email_allocation"
	accountEmailAllocationUpdatedEvent = "gpt.email_allocation.updated"
	credentialKindCodexPhone           = "codex_phone"
)

func newAccountHotStream(ctx context.Context, natsURL string) (hotstream.Bus, func(), error) {
	bus, err := hotstreamnats.Connect(ctx, hotstreamnats.Config{
		URL:        natsURL,
		ClientName: "gpt-account",
		Subject:    hotstream.ServiceStateSubject("gpt"),
	})
	if err != nil {
		return nil, nil, err
	}
	return bus, bus.Close, nil
}

func gptAccountProjection(account *pb.Account) *accountv1.Account {
	if account == nil {
		return nil
	}
	return gptAccountDescriptor.Account(
		gptAccountID(account),
		accountmodel.WithEmailIdentity(gptAccountEmail(account), gptAccountEmail(account)),
		accountmodel.WithStatus(gptAccountStatus(account)),
		accountmodel.WithCredentials(gptCredentialStates(account)...),
		accountmodel.WithCreatedTimestamp(account.GetAccount().GetCreatedAt()),
		accountmodel.WithUpdatedTimestamp(account.GetAccount().GetUpdatedAt()),
	)
}

func gptAccountStatus(account *pb.Account) *accountv1.AccountStatus {
	return accountmodel.StatusWithError(gptAccountStatusValue(account), "", gptAccountStateErrorCode, gptAccountErrorMessage(account), false)
}

func gptCredentialStates(account *pb.Account) []*accountv1.AccountCredentialState {
	states := []*accountv1.AccountCredentialState{}
	if credential := accountmodel.CredentialState(account.GetAccount(), accountmodel.CredentialKindMailbox); credential != nil {
		states = append(states, credential)
	} else {
		states = append(states, accountmodel.Credential(accountmodel.CredentialKindMailbox, account.GetPrimaryMailboxEmail() != "", accountmodel.CredentialStatusConfigured, time.Time{}, time.Time{}))
	}
	if credential := accountmodel.CredentialState(account.GetAccount(), credentialKindCodexPhone); credential != nil {
		states = append(states, credential)
	}
	return states
}

func (s *gptAccountServer) publishAllocationHotStream(ctx context.Context, allocation *pb.GPTEmailAllocation) {
	if s == nil || s.hot == nil || allocation == nil {
		return
	}
	updatedAt := allocation.GetUpdatedAt()
	if updatedAt <= 0 {
		updatedAt = time.Now().Unix()
	}
	event := hotstream.NewEvent(hotstream.EventConfig{
		EventID:       eventbus.StableEventID("gpt-email-allocation-", allocation.GetEmail(), allocation.GetStatus(), fmt.Sprintf("%d", updatedAt)),
		EventType:     accountEmailAllocationUpdatedEvent,
		SourceService: gptAccountDescriptor.SourceService,
		ResourceType:  accountAllocationResource,
		ResourceID:    allocation.GetEmail(),
		Scope:         allocation.GetStatus(),
		OccurredAt:    time.Unix(updatedAt, 0),
		CorrelationID: allocation.GetAssignedAccountId(),
		Attributes: map[string]string{
			"email":               allocation.GetEmail(),
			"primary_email":       allocation.GetPrimaryEmail(),
			"status":              allocation.GetStatus(),
			"assigned_account_id": allocation.GetAssignedAccountId(),
		},
	})
	if err := s.hot.Publish(context.WithoutCancel(ctx), event); err != nil {
		log.Printf("publish email allocation hotstream failed email=%s: %v", allocation.GetEmail(), err)
	}
}
