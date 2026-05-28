package jobqueue

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/byte-v-forge/common-lib/eventbus"
	"github.com/byte-v-forge/common-lib/eventoutbox"
	"gorm.io/gorm"

	"orchestrator/db"
)

type OutboxDispatcher struct{}

func NewOutboxDispatcher() *OutboxDispatcher {
	return &OutboxDispatcher{}
}

func (d *OutboxDispatcher) EnqueueJobAction(ctx context.Context, tx *gorm.DB, jobID string, action string, accountID string, reason string) error {
	if tx == nil {
		return fmt.Errorf("job action outbox transaction is required")
	}
	message, ok := ActionRequestedMessage(jobID, action, accountID, reason)
	if !ok {
		return nil
	}
	record, err := eventoutbox.NewRecord(message)
	if err != nil {
		return err
	}
	return eventoutbox.InsertRecordGORM(ctx, tx, db.PlatformEventOutboxTable, record, time.Now().Unix())
}

type OutboxWorker struct {
	db        *gorm.DB
	publisher eventbus.Publisher
}

func RunOutboxWorker(ctx context.Context, database *gorm.DB, publisher eventbus.Publisher) error {
	if database == nil || publisher == nil {
		return nil
	}
	return eventoutbox.RunWorker(ctx, eventoutbox.WorkerConfig{
		Name:      "GPT platform event outbox",
		Processor: &OutboxWorker{db: database, publisher: publisher},
		Logf:      logOutboxWarning,
	})
}

func (w *OutboxWorker) PublishPending(ctx context.Context, batch int) (int, error) {
	if w == nil || w.db == nil || w.publisher == nil {
		return 0, nil
	}
	if batch <= 0 {
		batch = eventoutbox.DefaultBatch
	}
	tx := w.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback().Error
		}
	}()

	rows, err := eventoutbox.ClaimPendingGORM(ctx, tx, db.PlatformEventOutboxTable, batch, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	updates, err := eventoutbox.NewGORMUpdates(tx, db.PlatformEventOutboxTable)
	if err != nil {
		return 0, err
	}
	published, err := eventoutbox.PublishRows(ctx, w.publisher, rows, updates, eventoutbox.PublishOptions{})
	if err != nil {
		return published, err
	}
	if err := tx.Commit().Error; err != nil {
		return published, err
	}
	committed = true
	return published, nil
}

func logOutboxWarning(format string, args ...any) {
	log.Printf("[orchestrator] "+format, args...)
}
