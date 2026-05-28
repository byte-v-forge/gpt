package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/byte-v-forge/common-lib/eventbus"
	"github.com/byte-v-forge/common-lib/hotstream"
	"github.com/byte-v-forge/common-lib/hotstreamnats"

	"gpt_account/pb"
)

const (
	accountHotStreamSource             = "gpt-account"
	accountResource                    = "gpt.account"
	accountAllocationResource          = "gpt.email_allocation"
	accountUpdatedEvent                = "gpt.account.updated"
	accountDeletedEvent                = "gpt.account.deleted"
	accountEmailAllocationUpdatedEvent = "gpt.email_allocation.updated"
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

func (s *gptAccountServer) publishAccountHotStream(ctx context.Context, eventType string, account *pb.Account) {
	if s == nil || s.hot == nil || account == nil {
		return
	}
	updatedAt := account.GetUpdatedAt()
	if updatedAt <= 0 {
		updatedAt = time.Now().Unix()
	}
	event := hotstream.NewEvent(hotstream.EventConfig{
		EventID:       eventbus.StableEventID("gpt-account-", eventType, account.GetAccountId(), fmt.Sprintf("%d", updatedAt)),
		EventType:     eventType,
		SourceService: accountHotStreamSource,
		ResourceType:  accountResource,
		ResourceID:    account.GetAccountId(),
		Scope:         account.GetStatus(),
		OccurredAt:    time.Unix(updatedAt, 0),
		CorrelationID: account.GetAccountId(),
		Attributes: map[string]string{
			"account_id": account.GetAccountId(),
			"status":     account.GetStatus(),
			"email":      account.GetEmail(),
		},
	})
	if err := s.hot.Publish(context.WithoutCancel(ctx), event); err != nil {
		log.Printf("publish account hotstream failed account=%s: %v", account.GetAccountId(), err)
	}
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
		SourceService: accountHotStreamSource,
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
