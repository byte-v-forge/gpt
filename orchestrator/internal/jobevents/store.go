package jobevents

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/eventbus"
	"github.com/byte-v-forge/common-lib/hotstream"
	"github.com/byte-v-forge/common-lib/protojsonx"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"

	"orchestrator/db"
	"orchestrator/pb"
)

const (
	GPTJobUpdatedEvent = "gpt.job.updated"
	GPTJobResource     = "gpt.job"
	GPTHotStreamSource = "gpt-orchestrator"
)

type Store struct {
	db  *gorm.DB
	hot hotstream.Publisher
}

func NewStore(database *gorm.DB, _ string) *Store {
	return &Store{db: database}
}

func (s *Store) WithHotStream(publisher hotstream.Publisher) *Store {
	s.hot = publisher
	return s
}

func (s *Store) Close() error { return nil }

func (s *Store) PublishSnapshot(ctx context.Context, eventType string, snapshot *pb.JobSnapshot) (*pb.JobEvent, error) {
	if s == nil || snapshot == nil || snapshot.GetJob() == nil {
		return nil, nil
	}
	jobID := strings.TrimSpace(snapshot.GetJob().GetJobId())
	if jobID == "" {
		return nil, nil
	}
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		eventType = "job_snapshot"
	}

	row := &db.JobEvent{JobID: jobID, EventType: eventType}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}

	snapshot.EventId = row.EventID
	data, err := (protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}).Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&db.JobEvent{}).
		Where("event_id = ?", row.EventID).
		Update("snapshot_json", string(data)).Error; err != nil {
		return nil, err
	}

	event := &pb.JobEvent{EventId: row.EventID, JobId: jobID, EventType: eventType, Snapshot: snapshot}
	s.publishHotStream(ctx, event)
	return event, nil
}

func (s *Store) Get(ctx context.Context, eventID int64) (*pb.JobEvent, error) {
	if s == nil {
		return nil, nil
	}
	var row db.JobEvent
	if err := s.db.WithContext(ctx).First(&row, "event_id = ?", eventID).Error; err != nil {
		return nil, err
	}
	return rowToProto(&row)
}

func (s *Store) publishHotStream(ctx context.Context, event *pb.JobEvent) {
	if s == nil || s.hot == nil || event == nil {
		return
	}
	job := event.GetSnapshot().GetJob()
	attrs := map[string]string{
		"job_id":     event.GetJobId(),
		"event_type": event.GetEventType(),
	}
	if job != nil {
		attrs["status"] = job.GetStatus()
		attrs["action"] = job.GetAction()
		attrs["account_id"] = job.GetAccountId()
	}
	hotEvent := hotstream.NewEvent(hotstream.EventConfig{
		EventID:       eventbus.StableEventID("gpt-job-", event.GetJobId(), fmt.Sprintf("%d", event.GetEventId())),
		EventType:     GPTJobUpdatedEvent,
		SourceService: GPTHotStreamSource,
		ResourceType:  GPTJobResource,
		ResourceID:    event.GetJobId(),
		Scope:         event.GetEventType(),
		OccurredAt:    time.Now(),
		CorrelationID: event.GetJobId(),
		Attributes:    attrs,
	})
	if err := s.hot.Publish(context.WithoutCancel(ctx), hotEvent); err != nil {
		log.Printf("[orchestrator] publish job hotstream failed job=%s: %v", event.GetJobId(), err)
	}
}

func rowToProto(row *db.JobEvent) (*pb.JobEvent, error) {
	if row == nil {
		return nil, nil
	}
	snapshot := &pb.JobSnapshot{}
	if strings.TrimSpace(row.SnapshotJSON) != "" {
		if err := protojsonx.Unmarshal([]byte(row.SnapshotJSON), snapshot); err != nil {
			return nil, err
		}
	}
	if snapshot.GetEventId() == 0 {
		snapshot.EventId = row.EventID
	}
	return &pb.JobEvent{EventId: row.EventID, JobId: row.JobID, EventType: row.EventType, Snapshot: snapshot}, nil
}
