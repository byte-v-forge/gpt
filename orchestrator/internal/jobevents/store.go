package jobevents

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"

	"orchestrator/db"
	"orchestrator/pb"
)

const notifyChannel = "job_events"

type Store struct {
	db     *gorm.DB
	dsn    string
	broker *broker
	cancel context.CancelFunc
}

type broker struct {
	mu   sync.Mutex
	subs map[chan *pb.JobEvent]struct{}
}

func NewStore(database *gorm.DB, dsn string) *Store {
	ctx, cancel := context.WithCancel(context.Background())
	store := &Store{
		db:     database,
		dsn:    dsn,
		broker: &broker{subs: map[chan *pb.JobEvent]struct{}{}},
		cancel: cancel,
	}
	go store.listen(ctx)
	return store
}

func (s *Store) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

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

	row := &db.JobEvent{
		JobID:     jobID,
		EventType: eventType,
	}
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

	event := &pb.JobEvent{
		EventId:   row.EventID,
		JobId:     jobID,
		EventType: eventType,
		Snapshot:  snapshot,
	}
	if err := s.notify(ctx, row.EventID); err != nil {
		log.Printf("[orchestrator] notify job event failed event=%d job=%s: %v", row.EventID, jobID, err)
	}
	s.broker.publish(event)
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

func (s *Store) Subscribe(ctx context.Context) (<-chan *pb.JobEvent, func()) {
	ch := make(chan *pb.JobEvent, 32)
	s.broker.subscribe(ch)
	cancel := func() {
		s.broker.unsubscribe(ch)
	}
	go func() {
		<-ctx.Done()
		cancel()
	}()
	return ch, cancel
}

func (s *Store) notify(ctx context.Context, eventID int64) error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.WithContext(ctx).Exec("SELECT pg_notify(?, ?)", notifyChannel, strconv.FormatInt(eventID, 10)).Error
}

func (s *Store) listen(ctx context.Context) {
	if s == nil || strings.TrimSpace(s.dsn) == "" {
		return
	}
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		if err := s.listenOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("[orchestrator] job event listener reconnecting: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}
}

func (s *Store) listenOnce(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, s.dsn)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())
	if _, err := conn.Exec(ctx, "LISTEN "+notifyChannel); err != nil {
		return err
	}
	for {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		eventID, err := strconv.ParseInt(strings.TrimSpace(notification.Payload), 10, 64)
		if err != nil || eventID <= 0 {
			continue
		}
		event, err := s.Get(ctx, eventID)
		if err != nil {
			log.Printf("[orchestrator] load job event failed event=%d: %v", eventID, err)
			continue
		}
		s.broker.publish(event)
	}
}

func rowToProto(row *db.JobEvent) (*pb.JobEvent, error) {
	if row == nil {
		return nil, nil
	}
	snapshot := &pb.JobSnapshot{}
	if strings.TrimSpace(row.SnapshotJSON) != "" {
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal([]byte(row.SnapshotJSON), snapshot); err != nil {
			return nil, err
		}
	}
	if snapshot.GetEventId() == 0 {
		snapshot.EventId = row.EventID
	}
	return &pb.JobEvent{
		EventId:   row.EventID,
		JobId:     row.JobID,
		EventType: row.EventType,
		Snapshot:  snapshot,
	}, nil
}

func (b *broker) subscribe(ch chan *pb.JobEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[ch] = struct{}{}
}

func (b *broker) unsubscribe(ch chan *pb.JobEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.subs[ch]; ok {
		delete(b.subs, ch)
		close(ch)
	}
}

func (b *broker) publish(event *pb.JobEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- event:
		default:
		}
	}
}
