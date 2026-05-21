package api

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm/clause"

	"orchestrator/db"
	"orchestrator/pb"
)

func (s *Server) GoPayUserSetWAPhone(ctx context.Context, req *pb.GoPayUserSetWAPhoneRequest) (*pb.GoPayUserWAPhoneResponse, error) {
	stateKey, err := normalizeGoPayUserID(req.GetUserId())
	if err != nil {
		return &pb.GoPayUserWAPhoneResponse{ErrorMessage: err.Error()}, nil
	}
	phone := normalizeIndonesiaPhoneForAPI(req.GetWaPhone())
	pin := strings.TrimSpace(req.GetPin())
	if phone == "" && pin == "" {
		return &pb.GoPayUserWAPhoneResponse{UserId: stateKey, ErrorMessage: "wa_phone or pin is required"}, nil
	}
	if s.db == nil {
		return &pb.GoPayUserWAPhoneResponse{UserId: stateKey, ErrorMessage: "orchestrator db not configured"}, nil
	}
	profile := db.GoPayUserProfile{StateKey: stateKey, WAPhone: phone, PIN: pin}
	updates := []string{"updated_at"}
	if phone != "" {
		updates = append(updates, "wa_phone")
	}
	if pin != "" {
		updates = append(updates, "pin")
	}
	err = s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "state_key"}},
		DoUpdates: clause.AssignmentColumns(updates),
	}).Create(&profile).Error
	if err != nil {
		return &pb.GoPayUserWAPhoneResponse{UserId: stateKey, ErrorMessage: fmt.Sprintf("save gopay user profile: %v", err)}, nil
	}
	return &pb.GoPayUserWAPhoneResponse{Success: true, UserId: stateKey, WaPhone: phone, Pin: pin}, nil
}

func (s *Server) GoPayUserGetWAPhone(ctx context.Context, req *pb.GoPayUserGetWAPhoneRequest) (*pb.GoPayUserWAPhoneResponse, error) {
	stateKey, err := normalizeGoPayUserID(req.GetUserId())
	if err != nil {
		return &pb.GoPayUserWAPhoneResponse{ErrorMessage: err.Error()}, nil
	}
	if s.db == nil {
		return &pb.GoPayUserWAPhoneResponse{UserId: stateKey, ErrorMessage: "orchestrator db not configured"}, nil
	}
	var profile db.GoPayUserProfile
	result := s.db.WithContext(ctx).Where("state_key = ?", stateKey).Limit(1).Find(&profile)
	if result.Error != nil {
		return &pb.GoPayUserWAPhoneResponse{UserId: stateKey, ErrorMessage: fmt.Sprintf("load wa_phone: %v", result.Error)}, nil
	}
	if result.RowsAffected == 0 {
		return &pb.GoPayUserWAPhoneResponse{Success: true, UserId: stateKey}, nil
	}
	return &pb.GoPayUserWAPhoneResponse{
		Success: true,
		UserId:  stateKey,
		WaPhone: normalizeIndonesiaPhoneForAPI(profile.WAPhone),
		Pin:     strings.TrimSpace(profile.PIN),
	}, nil
}

func normalizeIndonesiaPhoneForAPI(phone string) string {
	value := strings.TrimPrefix(strings.TrimSpace(phone), "+")
	if strings.HasPrefix(value, "62") {
		return strings.TrimPrefix(value[2:], "0")
	}
	return value
}
