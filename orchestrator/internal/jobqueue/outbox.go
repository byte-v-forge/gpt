package jobqueue

import (
	"context"
	"log"
	"time"

	"github.com/byte-v-forge/common-lib/eventbus"
	"github.com/byte-v-forge/common-lib/eventoutbox"
	"gorm.io/gorm"

	"orchestrator/db"
)

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
