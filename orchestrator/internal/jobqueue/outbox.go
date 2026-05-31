package jobqueue

import (
	"context"
	"log"

	"github.com/byte-v-forge/common-lib/eventbus"
	"github.com/byte-v-forge/common-lib/eventoutbox"
	"gorm.io/gorm"

	"orchestrator/db"
)

func RunOutboxWorker(ctx context.Context, database *gorm.DB, publisher eventbus.Publisher) error {
	if database == nil || publisher == nil {
		return nil
	}
	return eventoutbox.RunGORMWorker(ctx, eventoutbox.GORMWorkerConfig{
		Name:      "GPT platform event outbox",
		DB:        database,
		Table:     db.PlatformEventOutboxTable,
		Publisher: publisher,
		Logf:      logOutboxWarning,
	})
}

func logOutboxWarning(format string, args ...any) {
	log.Printf("[orchestrator] "+format, args...)
}
