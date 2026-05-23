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
	PIN       string `gorm:"column:pin"`
	CreatedAt int64  `gorm:"autoCreateTime"`
	UpdatedAt int64  `gorm:"autoUpdateTime"`
}

type RuntimeSecret struct {
	Key       string `gorm:"primaryKey;column:key"`
	Value     string `gorm:"column:value"`
	CreatedAt int64  `gorm:"autoCreateTime"`
	UpdatedAt int64  `gorm:"autoUpdateTime"`
}

type CodexOAuthPhoneLease struct {
	ActivationID       string `gorm:"primaryKey;column:activation_id"`
	PhoneE164          string `gorm:"column:phone_e164;index"`
	PhoneNational      string `gorm:"column:phone_national"`
	CountryISO2        string `gorm:"column:country_iso2;index"`
	CountryCallingCode string `gorm:"column:country_calling_code"`
	ProfileKey         string `gorm:"column:profile_key;index"`
	Status             string `gorm:"column:status;index"`
	Label              string `gorm:"column:label;index"`
	UseCount           int32  `gorm:"column:use_count"`
	MaxUseCount        int32  `gorm:"column:max_use_count"`
	ExpiresAt          int64  `gorm:"column:expires_at;index"`
	LastFailureKind    string `gorm:"column:last_failure_kind;index"`
	LastJobID          string `gorm:"column:last_job_id;index"`
	LastAccountID      string `gorm:"column:last_account_id;index"`
	LastError          string `gorm:"column:last_error"`
	CreatedAt          int64  `gorm:"autoCreateTime"`
	UpdatedAt          int64  `gorm:"autoUpdateTime"`
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

func (CodexOAuthPhoneLease) TableName() string {
	return "codex_oauth_phone_leases"
}

func DSN() string {
	dsn := strings.TrimSpace(os.Getenv("GPT_SERVICE_PG_DSN"))
	if dsn == "" {
		log.Fatal("GPT_SERVICE_PG_DSN is required")
	}
	return dsn
}

func InitDB() *gorm.DB {
	dsn := DSN()

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to PostgreSQL database: %v", err)
	}
	db.AutoMigrate(&Job{}, &JobParam{}, &JobStep{}, &JobEvent{}, &GoPayUserProfile{}, &RuntimeSecret{}, &CodexOAuthPhoneLease{})
	return db
}
