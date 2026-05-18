package db

import (
	"log"
	"os"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Job struct {
	ID           string `gorm:"primaryKey"`
	AccountID    string `gorm:"index"`
	Action       string `gorm:"index"`
	Status       string `gorm:"index"`
	Recoverable  bool
	Retryable    bool
	LastStep     string
	ErrorMessage string
	ResultJSON   string
	CreatedAt    int64 `gorm:"autoCreateTime"`
	UpdatedAt    int64 `gorm:"autoUpdateTime"`
}

type JobParam struct {
	JobID     string `gorm:"primaryKey"`
	Key       string `gorm:"primaryKey"`
	Value     string
	CreatedAt int64 `gorm:"autoCreateTime"`
	UpdatedAt int64 `gorm:"autoUpdateTime"`
}

type JobStep struct {
	JobID        string `gorm:"primaryKey"`
	StepName     string `gorm:"primaryKey;column:step_name"`
	Status       string `gorm:"index"`
	Recoverable  bool
	Retryable    bool
	ErrorMessage string
	ResultJSON   string
	StartedAt    int64
	CompletedAt  int64
	CreatedAt    int64 `gorm:"autoCreateTime"`
	UpdatedAt    int64 `gorm:"autoUpdateTime"`
}

type JobEvent struct {
	EventID      int64  `gorm:"primaryKey;autoIncrement;column:event_id"`
	JobID        string `gorm:"index"`
	EventType    string `gorm:"index"`
	SnapshotJSON string
	CreatedAt    int64 `gorm:"autoCreateTime"`
}

type GoPayUserProfile struct {
	StateKey  string `gorm:"primaryKey;column:state_key"`
	WAPhone   string `gorm:"column:wa_phone"`
	CreatedAt int64  `gorm:"autoCreateTime"`
	UpdatedAt int64  `gorm:"autoUpdateTime"`
}

type RuntimeSecret struct {
	Key       string `gorm:"primaryKey;column:key"`
	Value     string `gorm:"column:value"`
	CreatedAt int64  `gorm:"autoCreateTime"`
	UpdatedAt int64  `gorm:"autoUpdateTime"`
}

func (JobEvent) TableName() string {
	return "job_events"
}

func (GoPayUserProfile) TableName() string {
	return "gopay_user_profiles"
}

func (RuntimeSecret) TableName() string {
	return "runtime_secrets"
}

func DSN() string {
	dsn := strings.TrimSpace(os.Getenv("ORCHESTRATOR_PG_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("PG_DSN"))
	}
	if dsn == "" {
		log.Fatal("ORCHESTRATOR_PG_DSN or PG_DSN is required")
	}
	return dsn
}

func InitDB() *gorm.DB {
	dsn := DSN()

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to PostgreSQL database: %v", err)
	}
	db.AutoMigrate(&Job{}, &JobParam{}, &JobStep{}, &JobEvent{}, &GoPayUserProfile{}, &RuntimeSecret{})
	return db
}
