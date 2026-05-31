package gptsettings

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

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
			Enabled:                false,
			RequireResidential:     proto.Bool(true),
			MinIpPurityScore:       90,
			CfCanaryEnabled:        proto.Bool(true),
			MaxProxyAttempts:       10,
			TargetConnectivityUrls: []string{"https://api.openai.com/v1/models"},
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
		out.ProxyPreflight.MinIpPurityScore = clampPurityScore(preflight.GetMinIpPurityScore())
		if preflight.GetMaxProxyAttempts() > 0 {
			out.ProxyPreflight.MaxProxyAttempts = preflight.GetMaxProxyAttempts()
		}
		if urls := normalizeTargetConnectivityURLs(preflight.GetTargetConnectivityUrls()); len(urls) > 0 {
			out.ProxyPreflight.TargetConnectivityUrls = urls
		}
	}
	out.PluginConfigs = normalizePluginConfigs(in.GetPluginConfigs())
	return out
}

func PluginValues(settings *pb.GPTSettings, pluginKey string) map[string]string {
	pluginKey = normalizePluginKey(pluginKey)
	if pluginKey == "" || settings == nil {
		return nil
	}
	for _, cfg := range settings.GetPluginConfigs() {
		if normalizePluginKey(cfg.GetPluginKey()) != pluginKey {
			continue
		}
		out := make(map[string]string, len(cfg.GetValues()))
		for key, value := range cfg.GetValues() {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			out[key] = strings.TrimSpace(value)
		}
		return out
	}
	return nil
}

func MergePluginValues(defaults map[string]string, overrides map[string]string) map[string]string {
	if len(defaults) == 0 && len(overrides) == 0 {
		return nil
	}
	out := make(map[string]string, len(defaults)+len(overrides))
	for key, value := range defaults {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	for key, value := range overrides {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func StringValue(values map[string]string, key string, fallback string) string {
	if values == nil {
		return fallback
	}
	value := strings.TrimSpace(values[strings.TrimSpace(key)])
	if value == "" {
		return fallback
	}
	return value
}

func BoolValue(values map[string]string, key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(values[strings.TrimSpace(key)]))
	switch value {
	case "true", "1", "yes", "y", "on":
		return true
	case "false", "0", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func IntValue(values map[string]string, key string, fallback int) int {
	value := strings.TrimSpace(values[strings.TrimSpace(key)])
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func Int32Value(values map[string]string, key string, fallback int32) int32 {
	parsed := IntValue(values, key, int(fallback))
	return int32(parsed)
}

func DurationSecondsValue(values map[string]string, key string, fallback time.Duration) time.Duration {
	seconds := IntValue(values, key, int(fallback/time.Second))
	if seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func normalizePluginConfigs(values []*pb.GPTPluginConfig) []*pb.GPTPluginConfig {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]*pb.GPTPluginConfig, 0, len(values))
	for _, cfg := range values {
		pluginKey := normalizePluginKey(cfg.GetPluginKey())
		if pluginKey == "" || seen[pluginKey] {
			continue
		}
		normalizedValues := map[string]string{}
		for key, value := range cfg.GetValues() {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			normalizedValues[key] = strings.TrimSpace(value)
		}
		seen[pluginKey] = true
		out = append(out, &pb.GPTPluginConfig{PluginKey: pluginKey, Values: normalizedValues})
	}
	return out
}

func normalizePluginKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func clampPurityScore(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func normalizeTargetConnectivityURLs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
