package db

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"math/big"
	"os"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Account struct {
	ID                             string `gorm:"primaryKey"`
	Email                          string `gorm:"uniqueIndex;not null"`
	Password                       string
	Status                         string
	ErrorMessage                   string
	SessionToken                   string
	AccessToken                    string
	ChargeRef                      string
	FirstName                      string
	LastName                       string
	DOB                            string // YYYY-MM-DD
	PlusTrialEligible              *bool
	PlusActive                     *bool
	Tier                           string
	ActivationChannel              string
	PrimaryMailboxEmail            string `gorm:"index"`
	MailboxLastFetchedAtUnix       int64
	MailboxLastMessageAtUnix       int64
	MailboxLatestOTP               string
	MailboxLatestOTPSubject        string
	MailboxLatestOTPReceivedAtUnix int64
	CodexAuthJSON                  string `gorm:"column:codex_auth_json"`
	CodexAuthUpdatedAtUnix         int64  `gorm:"column:codex_auth_updated_at_unix"`
	CodexPhoneConfirmed            *bool  `gorm:"column:codex_phone_confirmed"`
	CodexPhoneLabel                string `gorm:"column:codex_phone_label"`
	CodexPhoneUpdatedAtUnix        int64  `gorm:"column:codex_phone_updated_at_unix"`
	CreatedAt                      int64  `gorm:"autoCreateTime"`
	UpdatedAt                      int64  `gorm:"autoUpdateTime"`
}

type GPTEmailAllocation struct {
	Email             string `gorm:"primaryKey"`
	PrimaryEmail      string `gorm:"index"`
	IsPrimary         bool   `gorm:"index"`
	Status            string `gorm:"index"`
	Splittable        bool   `gorm:"index"`
	AssignedAccountID string `gorm:"index"`
	LastError         string
	CreatedAt         int64 `gorm:"autoCreateTime"`
	UpdatedAt         int64 `gorm:"autoUpdateTime"`
}

func InitDB() *gorm.DB {
	dsn := strings.TrimSpace(os.Getenv("PG_DSN"))
	if dsn == "" {
		log.Fatal("PG_DSN is required")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to PostgreSQL database: %v", err)
	}

	db.AutoMigrate(&Account{}, &GPTEmailAllocation{})
	if err := db.Exec("DELETE FROM accounts WHERE email IS NULL OR email = ''").Error; err != nil {
		log.Printf("failed to delete invalid accounts without email: %v", err)
	}
	if err := db.Exec("ALTER TABLE accounts ALTER COLUMN email SET NOT NULL").Error; err != nil {
		log.Printf("failed to enforce accounts.email NOT NULL: %v", err)
	}
	if err := db.Exec("ALTER TABLE accounts DROP COLUMN IF EXISTS proxy_url").Error; err != nil {
		log.Printf("failed to drop legacy proxy_url column: %v", err)
	}
	if err := backfillAccountPrimaryMailboxes(db); err != nil {
		log.Printf("failed to backfill account primary mailboxes: %v", err)
	}
	if err := backfillGPTEmailAllocationsFromAccounts(db); err != nil {
		log.Printf("failed to backfill GPT email allocations from accounts: %v", err)
	}
	if err := backfillCodexAuthJSONFromRuntimeSecrets(db); err != nil {
		log.Printf("failed to backfill codex auth json from runtime secrets: %v", err)
	}
	if err := backfillCodexPhoneConfirmed(db); err != nil {
		log.Printf("failed to backfill codex phone state: %v", err)
	}

	return db
}

func backfillAccountPrimaryMailboxes(db *gorm.DB) error {
	return db.Exec(`
		UPDATE accounts
		SET primary_mailbox_email = CASE
			WHEN lower(btrim(email)) LIKE '%@%' THEN split_part(split_part(lower(btrim(email)), '@', 1), '+', 1) || '@' || split_part(lower(btrim(email)), '@', 2)
			ELSE lower(btrim(email))
		END
		WHERE email IS NOT NULL
			AND btrim(email) <> ''
			AND COALESCE(primary_mailbox_email, '') = ''
	`).Error
}

func backfillGPTEmailAllocationsFromAccounts(db *gorm.DB) error {
	if err := db.Exec(`
		WITH account_allocations AS (
			SELECT
				lower(btrim(email)) AS email,
				CASE
					WHEN lower(btrim(email)) LIKE '%@%' THEN split_part(split_part(lower(btrim(email)), '@', 1), '+', 1) || '@' || split_part(lower(btrim(email)), '@', 2)
					ELSE lower(btrim(email))
				END AS primary_email,
				id AS account_id,
				CASE
					WHEN status IN ('USER_ALREADY_EXISTS', 'EMAIL_ALREADY_EXISTS') THEN 'USER_ALREADY_EXISTS'
					WHEN status IN ('REGISTERED', 'ACTIVATED') OR session_token <> '' OR access_token <> '' THEN 'REGISTERED'
					WHEN status IN ('REGISTER_FAILED') THEN 'REGISTRATION_FAILED'
					ELSE 'ASSIGNED'
				END AS allocation_status,
				error_message,
				created_at,
				updated_at
			FROM accounts
			WHERE email IS NOT NULL AND btrim(email) <> ''
		)
		INSERT INTO gpt_email_allocations (
			email, primary_email, is_primary, status, splittable,
			assigned_account_id, last_error, created_at, updated_at
		)
		SELECT
			email,
			primary_email,
			email = primary_email,
			allocation_status,
			allocation_status = 'REGISTERED' AND email = primary_email,
			account_id,
			COALESCE(error_message, ''),
			created_at,
			updated_at
		FROM account_allocations
		ON CONFLICT (email) DO UPDATE SET
			primary_email = EXCLUDED.primary_email,
			is_primary = EXCLUDED.is_primary,
			assigned_account_id = EXCLUDED.assigned_account_id,
			status = CASE
				WHEN EXCLUDED.status = 'USER_ALREADY_EXISTS' THEN EXCLUDED.status
				WHEN EXCLUDED.status = 'REGISTERED' AND gpt_email_allocations.status NOT IN ('USER_ALREADY_EXISTS', 'BLOCKED') THEN EXCLUDED.status
				WHEN EXCLUDED.status = 'REGISTRATION_FAILED' AND gpt_email_allocations.status NOT IN ('USER_ALREADY_EXISTS', 'BLOCKED', 'REGISTERED') THEN EXCLUDED.status
				WHEN EXCLUDED.status = 'ASSIGNED' AND gpt_email_allocations.status IN ('', 'AVAILABLE', 'OAUTH_PENDING', 'AUTH_FAILED', 'NEEDS_MANUAL_VERIFICATION') THEN EXCLUDED.status
				ELSE gpt_email_allocations.status
			END,
			splittable = CASE
				WHEN EXCLUDED.status = 'REGISTERED' AND EXCLUDED.is_primary AND gpt_email_allocations.status NOT IN ('USER_ALREADY_EXISTS', 'BLOCKED') THEN true
				WHEN EXCLUDED.status IN ('USER_ALREADY_EXISTS', 'BLOCKED') THEN false
				ELSE gpt_email_allocations.splittable
			END,
			last_error = CASE
				WHEN EXCLUDED.last_error <> '' THEN EXCLUDED.last_error
				ELSE gpt_email_allocations.last_error
			END,
			updated_at = GREATEST(gpt_email_allocations.updated_at, EXCLUDED.updated_at)
	`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		UPDATE gpt_email_allocations AS primary_row
		SET status = 'REGISTERED',
			splittable = true,
			updated_at = EXTRACT(EPOCH FROM NOW())::BIGINT
		FROM gpt_email_allocations AS child_row
		WHERE child_row.status = 'REGISTERED'
			AND child_row.primary_email = primary_row.email
			AND primary_row.is_primary = true
			AND primary_row.status NOT IN ('USER_ALREADY_EXISTS', 'BLOCKED')
	`).Error; err != nil {
		return err
	}
	if err := db.Exec(`
		UPDATE gpt_email_allocations AS primary_row
		SET status = 'BLOCKED',
			splittable = false,
			updated_at = EXTRACT(EPOCH FROM NOW())::BIGINT
		FROM gpt_email_allocations AS child_row
		WHERE child_row.status = 'USER_ALREADY_EXISTS'
			AND child_row.primary_email = primary_row.email
			AND child_row.email <> primary_row.email
			AND primary_row.is_primary = true
			AND primary_row.status <> 'USER_ALREADY_EXISTS'
	`).Error; err != nil {
		return err
	}
	return db.Exec(`
		UPDATE gpt_email_allocations AS alias_row
		SET status = 'BLOCKED',
			splittable = false,
			updated_at = EXTRACT(EPOCH FROM NOW())::BIGINT
		FROM gpt_email_allocations AS primary_row
		WHERE primary_row.status IN ('USER_ALREADY_EXISTS', 'BLOCKED')
			AND alias_row.primary_email = primary_row.email
			AND alias_row.is_primary = false
			AND alias_row.status = 'AVAILABLE'
	`).Error
}

func backfillCodexPhoneConfirmed(db *gorm.DB) error {
	return db.Exec(`
		WITH ranked AS (
			SELECT id, row_number() OVER (ORDER BY created_at DESC, id DESC) AS rn
			FROM accounts
		)
		UPDATE accounts
		SET
			codex_phone_confirmed = true,
			codex_phone_label = CASE WHEN COALESCE(codex_phone_label, '') = '' THEN 'backfill-existing' ELSE codex_phone_label END,
			codex_phone_updated_at_unix = CASE WHEN COALESCE(codex_phone_updated_at_unix, 0) = 0 THEN EXTRACT(EPOCH FROM NOW())::BIGINT ELSE codex_phone_updated_at_unix END
		FROM ranked
		WHERE accounts.id = ranked.id
			AND ranked.rn > 3
			AND codex_phone_confirmed IS NULL
	`).Error
}

func backfillCodexAuthJSONFromRuntimeSecrets(db *gorm.DB) error {
	return db.Exec(`
		DO $$
		BEGIN
			IF to_regclass('public.runtime_secrets') IS NOT NULL THEN
				UPDATE accounts
				SET
					codex_auth_json = runtime_secrets.value,
					codex_auth_updated_at_unix = GREATEST(COALESCE(accounts.codex_auth_updated_at_unix, 0), runtime_secrets.updated_at)
				FROM runtime_secrets
				WHERE runtime_secrets.key = 'codex_oauth_auth_json:' || accounts.id
					AND COALESCE(accounts.codex_auth_json, '') = ''
					AND COALESCE(runtime_secrets.value, '') <> '';
			END IF;
		END $$;
	`).Error
}

func RandomAliasEmail(primary string, length int) (string, error) {
	local, domain, ok := strings.Cut(NormalizeEmail(primary), "@")
	if !ok || local == "" || domain == "" {
		return "", nil
	}
	token, err := randomAliasToken(length)
	if err != nil {
		return "", err
	}
	return local + "+" + token + "@" + domain, nil
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func randomAliasToken(length int) (string, error) {
	if length <= 0 {
		length = 6
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	var b strings.Builder
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b.WriteByte(alphabet[n.Int64()])
	}
	return b.String(), nil
}

func RandomID(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
