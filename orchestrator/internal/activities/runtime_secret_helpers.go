package activities

import (
	"context"
	"strings"

	"gorm.io/gorm/clause"
	"orchestrator/db"
)

func (s *Server) loadRekberinajaTokens(ctx context.Context, fallbackAccessToken string, fallbackRefreshToken string) (string, string) {
	if s == nil || s.db == nil {
		return fallbackAccessToken, fallbackRefreshToken
	}
	accessToken := fallbackAccessToken
	refreshToken := fallbackRefreshToken
	var rows []db.RuntimeSecret
	if err := s.db.WithContext(ctx).Where("key IN ?", []string{rekberinajaAccessTokenSecretKey, rekberinajaRefreshTokenSecretKey}).Find(&rows).Error; err != nil {
		return accessToken, refreshToken
	}
	for _, row := range rows {
		switch row.Key {
		case rekberinajaAccessTokenSecretKey:
			if value := strings.TrimSpace(row.Value); value != "" {
				accessToken = value
			}
		case rekberinajaRefreshTokenSecretKey:
			if value := strings.TrimSpace(row.Value); value != "" {
				refreshToken = value
			}
		}
	}
	return accessToken, refreshToken
}

func (s *Server) loadRuntimeSecret(ctx context.Context, key string) string {
	if s == nil || s.db == nil || strings.TrimSpace(key) == "" {
		return ""
	}
	var row db.RuntimeSecret
	if err := s.db.WithContext(ctx).First(&row, "key = ?", strings.TrimSpace(key)).Error; err != nil {
		return ""
	}
	return strings.TrimSpace(row.Value)
}

func (s *Server) saveRekberinajaTokens(ctx context.Context, accessToken string, refreshToken string) error {
	if s == nil || s.db == nil {
		return nil
	}
	if strings.TrimSpace(accessToken) != "" {
		if err := s.saveRuntimeSecret(ctx, rekberinajaAccessTokenSecretKey, accessToken); err != nil {
			return err
		}
	}
	if strings.TrimSpace(refreshToken) != "" {
		if err := s.saveRuntimeSecret(ctx, rekberinajaRefreshTokenSecretKey, refreshToken); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) saveRuntimeSecret(ctx context.Context, key string, value string) error {
	if s == nil || s.db == nil || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
		return nil
	}
	row := db.RuntimeSecret{Key: strings.TrimSpace(key), Value: strings.TrimSpace(value)}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&row).Error
}
