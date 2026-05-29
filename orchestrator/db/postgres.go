package db

import (
	"log"
	"os"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const PlatformEventOutboxTable = "gpt_platform_event_outbox"

type Job struct {
	ID             string `gorm:"primaryKey"`
	AccountID      string `gorm:"index"`
	Action         string `gorm:"index"`
	Status         string `gorm:"index"`
	Recoverable    bool
	Retryable      bool
	LastStep       string
	ErrorMessage   string
	ResultJSON     string
	ClaimOwner     string `gorm:"index"`
	ClaimUntil     int64  `gorm:"index"`
	AttemptCount   int32
	CreatedAt      int64  `gorm:"autoCreateTime"`
	UpdatedAt      int64  `gorm:"autoUpdateTime"`
	N8NExecutionID string `gorm:"column:n8n_execution_id;index"`
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

type GPTRuntimeSetting struct {
	SettingsKey string `gorm:"primaryKey;column:settings_key"`
	ValueJSON   string `gorm:"column:value_json"`
	CreatedAt   int64  `gorm:"autoCreateTime"`
	UpdatedAt   int64  `gorm:"autoUpdateTime"`
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

type AccountProxyUsage struct {
	ID             string `gorm:"primaryKey;column:id"`
	AccountID      string `gorm:"column:account_id;index"`
	JobID          string `gorm:"column:job_id;index"`
	N8NExecutionID string `gorm:"column:n8n_execution_id;index"`
	Purpose        string `gorm:"column:purpose;index"`
	ProxyURLHash   string `gorm:"column:proxy_url_hash;index"`
	SessionIDHash  string `gorm:"column:session_id_hash;index"`
	ExitIP         string `gorm:"column:exit_ip;index"`
	CountryCode    string `gorm:"column:country_code;index"`
	Region         string `gorm:"column:region"`
	City           string `gorm:"column:city"`
	AttemptIndex   uint32 `gorm:"column:attempt_index"`
	Accepted       bool   `gorm:"column:accepted;index"`
	ErrorMessage   string `gorm:"column:error_message"`
	RawJSON        string `gorm:"column:raw_json"`
	CreatedAt      int64  `gorm:"autoCreateTime"`
}

type AccountBrowserFingerprint struct {
	AccountID              string `gorm:"primaryKey;column:account_id"`
	CountryCode            string `gorm:"column:country_code;index"`
	Region                 string `gorm:"column:region"`
	BrowserProfileTemplate string `gorm:"column:browser_profile_template;index"`
	BrowserFamily          string `gorm:"column:browser_family"`
	BrowserMajorVersion    string `gorm:"column:browser_major_version"`
	OSFamily               string `gorm:"column:os_family"`
	TLSProfileFamily       string `gorm:"column:tls_profile_family;index"`
	TLSFingerprintVariant  string `gorm:"column:tls_fingerprint_variant;index"`
	Locale                 string `gorm:"column:locale"`
	Timezone               string `gorm:"column:timezone"`
	UserAgent              string `gorm:"column:user_agent"`
	AcceptLanguage         string `gorm:"column:accept_language"`
	Language               string `gorm:"column:language"`
	DeviceID               string `gorm:"column:device_id"`
	CreatedAt              int64  `gorm:"autoCreateTime"`
	UpdatedAt              int64  `gorm:"autoUpdateTime"`
}

func (AccountProxyUsage) TableName() string {
	return "account_proxy_usages"
}

func (JobEvent) TableName() string {
	return "job_events"
}

func (GoPayUserProfile) TableName() string {
	return "gopay_user_profiles"
}

func (GPTRuntimeSetting) TableName() string {
	return "gpt_runtime_settings"
}

func (CodexOAuthPhoneLease) TableName() string {
	return "codex_oauth_phone_leases"
}

func (AccountBrowserFingerprint) TableName() string {
	return "account_browser_fingerprints"
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
	validateSchema(db)
	return db
}

func validateSchema(db *gorm.DB) {
	for _, table := range []string{
		"jobs",
		"job_params",
		"job_steps",
		"job_events",
		"gopay_user_profiles",
		"gpt_runtime_settings",
		"codex_oauth_phone_leases",
		"account_browser_fingerprints",
		PlatformEventOutboxTable,
	} {
		if !db.Migrator().HasTable(table) {
			log.Fatalf("database schema is not migrated: missing table %s", table)
		}
	}
}
