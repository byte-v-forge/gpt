package db

import (
	"log"
	"os"
	"strings"

	"github.com/byte-v-forge/common-lib/emailx"
	"github.com/byte-v-forge/common-lib/randx"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Account struct {
	ID        string `gorm:"primaryKey"`
	Email     string `gorm:"uniqueIndex;not null"`
	Password  string
	CreatedAt int64 `gorm:"autoCreateTime"`
	UpdatedAt int64 `gorm:"autoUpdateTime"`
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

	validateSchema(db)
	return db
}

func validateSchema(db *gorm.DB) {
	for _, table := range []string{"accounts", "gpt_email_allocations"} {
		if !db.Migrator().HasTable(table) {
			log.Fatalf("database schema is not migrated: missing table %s", table)
		}
	}
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
	return emailx.Normalize(email)
}

func randomAliasToken(length int) (string, error) {
	if length <= 0 {
		length = 6
	}
	return randx.String(randx.AlphabetLowerNum, length)
}
