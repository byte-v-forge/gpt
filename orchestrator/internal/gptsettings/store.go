package gptsettings

import (
	"context"
	"fmt"
	"strings"

	"github.com/byte-v-forge/common-lib/gormx"
	"github.com/byte-v-forge/common-lib/protojsonx"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"

	"orchestrator/db"
	"orchestrator/pb"
)

const globalSettingsKey = "global"

type Store struct {
	db *gorm.DB
}

func NewStore(database *gorm.DB) *Store {
	return &Store{db: database}
}

func (s *Store) Get(ctx context.Context) (*pb.GPTSettings, error) {
	if s == nil || s.db == nil {
		return Default(), nil
	}
	var row db.GPTRuntimeSetting
	result := s.db.WithContext(ctx).Where("settings_key = ?", globalSettingsKey).Limit(1).Find(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("load GPT settings: %w", result.Error)
	}
	if result.RowsAffected == 0 || strings.TrimSpace(row.ValueJSON) == "" {
		return Default(), nil
	}
	settings := &pb.GPTSettings{}
	if err := protojsonx.Unmarshal([]byte(row.ValueJSON), settings); err != nil {
		return nil, fmt.Errorf("decode GPT settings: %w", err)
	}
	return Normalize(settings), nil
}

func (s *Store) Update(ctx context.Context, settings *pb.GPTSettings) (*pb.GPTSettings, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("GPT settings store is not configured")
	}
	normalized := Normalize(settings)
	raw, err := protojsonx.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode GPT settings: %w", err)
	}
	row := db.GPTRuntimeSetting{SettingsKey: globalSettingsKey, ValueJSON: string(raw)}
	err = s.db.WithContext(ctx).
		Clauses(gormx.OnConflictUpdateColumns([]string{"settings_key"}, []string{"value_json", "updated_at"})).
		Create(&row).Error
	if err != nil {
		return nil, fmt.Errorf("save GPT settings: %w", err)
	}
	return normalized, nil
}

func Default() *pb.GPTSettings {
	return &pb.GPTSettings{
		ProxyPreflight: &pb.GPTProxyPreflightSettings{
			Enabled:            false,
			RequireResidential: proto.Bool(true),
			MinIpPurityScore:   90,
			CfCanaryEnabled:    proto.Bool(true),
			MaxProxyAttempts:   10,
		},
	}
}

func Normalize(in *pb.GPTSettings) *pb.GPTSettings {
	out := proto.Clone(Default()).(*pb.GPTSettings)
	if in == nil {
		return out
	}
	if preflight := in.GetProxyPreflight(); preflight != nil {
		out.ProxyPreflight.Enabled = preflight.GetEnabled()
		if preflight.RequireResidential != nil {
			out.ProxyPreflight.RequireResidential = proto.Bool(preflight.GetRequireResidential())
		}
		if preflight.CfCanaryEnabled != nil {
			out.ProxyPreflight.CfCanaryEnabled = proto.Bool(preflight.GetCfCanaryEnabled())
		}
		if score := preflight.GetMinIpPurityScore(); score > 0 {
			out.ProxyPreflight.MinIpPurityScore = clampPurityScore(score)
		}
		if preflight.GetMaxProxyAttempts() > 0 {
			out.ProxyPreflight.MaxProxyAttempts = preflight.GetMaxProxyAttempts()
		}
	}
	return out
}

func clampPurityScore(score float64) float64 {
	if score < 90 {
		return 90
	}
	if score > 100 {
		return 100
	}
	return score
}
