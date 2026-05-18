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

type Filter struct {
	JobIDs       []string
	AfterEventID int64
	Limit        int
}

type broker struct {
	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

func NewStore(database *gorm.DB, dsn string) *Store {
	ctx, cancel := context.WithCancel(context.Background())
	store := &Store{
		db:     database,
		dsn:    dsn,
		broker: &broker{subs: map[chan struct{}]struct{}{}},
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
	s.broker.publish()
	return event, nil
}

func (s *Store) List(ctx context.Context, filter Filter) ([]*pb.JobEvent, error) {
	if s == nil {
		return nil, nil
	}
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	query := s.db.WithContext(ctx).Model(&db.JobEvent{})
	if filter.AfterEventID > 0 {
		query = query.Where("event_id > ?", filter.AfterEventID)
	}
	if len(filter.JobIDs) > 0 {
		query = query.Where("job_id IN ?", compactStrings(filter.JobIDs))
	}

	var rows []db.JobEvent
	if err := query.Order("event_id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	events := make([]*pb.JobEvent, 0, len(rows))
	for i := range rows {
		event, err := rowToProto(&rows[i])
		if err != nil {
			log.Printf("[orchestrator] decode job event failed event=%d job=%s: %v", rows[i].EventID, rows[i].JobID, err)
			continue
		}
		events = append(events, event)
	}
	return events, nil
}

func (s *Store) Subscribe(ctx context.Context) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
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
		if _, err := conn.WaitForNotification(ctx); err != nil {
			return err
		}
		s.broker.publish()
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

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func (b *broker) subscribe(ch chan struct{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[ch] = struct{}{}
}

func (b *broker) unsubscribe(ch chan struct{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.subs[ch]; ok {
		delete(b.subs, ch)
		close(ch)
	}
}

func (b *broker) publish() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
