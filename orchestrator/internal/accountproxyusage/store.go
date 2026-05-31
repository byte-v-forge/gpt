package accountproxyusage

import (
	"context"
	"fmt"
	"strings"

	"github.com/byte-v-forge/common-lib/hashx"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"

	"orchestrator/db"
	"orchestrator/pb"
)

type Store struct {
	db *gorm.DB
}

type RecordInput struct {
	JobID          string
	AccountID      string
	N8NExecutionID string
	Purpose        string
	ProxyURL       string
	Data           *pb.N8NDynamicProxyPreflightData
}

type Usage struct {
	ID             string
	AccountID      string
	JobID          string
	N8NExecutionID string
	Purpose        string
	ProxyURLHash   string
	SessionIDHash  string
	ExitIP         string
	CountryCode    string
	Region         string
	City           string
	AttemptIndex   uint32
	Accepted       bool
	ErrorMessage   string
	RawJSON        string
	CreatedAt      int64
}

func NewStore(database *gorm.DB) *Store {
	return &Store{db: database}
}

func (s *Store) Record(ctx context.Context, input RecordInput) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("account proxy usage store is not configured")
	}
	if input.Data == nil {
		return fmt.Errorf("dynamic proxy preflight data is required")
	}
	row := rowFromRecord(input)
	return s.db.WithContext(ctx).Create(&row).Error
}

func (s *Store) ListByAccount(ctx context.Context, accountID string, limit int) ([]Usage, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("account proxy usage store is not configured")
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, fmt.Errorf("account_id is required")
	}
	if limit <= 0 {
		limit = 50
	}
	var rows []db.AccountProxyUsage
	if err := s.db.WithContext(ctx).
		Where("account_id = ?", accountID).
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Usage, 0, len(rows))
	for i := range rows {
		out = append(out, usageFromRow(rows[i]))
	}
	return out, nil
}

func rowFromRecord(input RecordInput) db.AccountProxyUsage {
	return db.AccountProxyUsage{
		ID:             uuid.NewString(),
		AccountID:      strings.TrimSpace(input.AccountID),
		JobID:          strings.TrimSpace(input.JobID),
		N8NExecutionID: strings.TrimSpace(input.N8NExecutionID),
		Purpose:        purpose(input.Purpose, input.Data),
		ProxyURLHash:   hashx.ShortSHA256(strings.TrimSpace(input.ProxyURL), 16),
		SessionIDHash:  sessionIDHash(input.Data),
		ExitIP:         exitIP(input.Data),
		CountryCode:    input.Data.GetExitGeo().GetCountryCode(),
		Region:         input.Data.GetExitGeo().GetRegion(),
		City:           input.Data.GetExitGeo().GetCity(),
		AttemptIndex:   input.Data.GetAttempt(),
		Accepted:       input.Data.GetAccepted(),
		ErrorMessage:   input.Data.GetErrorMessage(),
		RawJSON:        recordRawJSON(input.Data),
	}
}

func recordRawJSON(data *pb.N8NDynamicProxyPreflightData) string {
	if data == nil {
		return ""
	}
	raw, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(data)
	if err != nil {
		return ""
	}
	return string(raw)
}

func usageFromRow(row db.AccountProxyUsage) Usage {
	return Usage{
		ID:             row.ID,
		AccountID:      row.AccountID,
		JobID:          row.JobID,
		N8NExecutionID: row.N8NExecutionID,
		Purpose:        row.Purpose,
		ProxyURLHash:   row.ProxyURLHash,
		SessionIDHash:  row.SessionIDHash,
		ExitIP:         row.ExitIP,
		CountryCode:    row.CountryCode,
		Region:         row.Region,
		City:           row.City,
		AttemptIndex:   row.AttemptIndex,
		Accepted:       row.Accepted,
		ErrorMessage:   row.ErrorMessage,
		RawJSON:        row.RawJSON,
		CreatedAt:      row.CreatedAt,
	}
}

func purpose(value string, data *pb.N8NDynamicProxyPreflightData) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return strings.TrimSpace(data.GetPurpose())
}

func sessionIDHash(data *pb.N8NDynamicProxyPreflightData) string {
	if data == nil {
		return ""
	}
	return strings.TrimSpace(data.GetProxySessionHash())
}

func exitIP(data *pb.N8NDynamicProxyPreflightData) string {
	if data == nil {
		return ""
	}
	if value := strings.TrimSpace(data.GetExitIp()); value != "" {
		return value
	}
	return strings.TrimSpace(data.GetExitGeo().GetIp())
}
