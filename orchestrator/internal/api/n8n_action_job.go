package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/byte-v-forge/common-lib/emailx"
	"github.com/google/uuid"

	"orchestrator/internal/contracts"
)

const (
	n8nEngineValue                 = "n8n"
	n8nDefaultEngineParam          = "engine"
	n8nDefaultAccountIDParam       = "account_id"
	n8nTargetConnectivityURLsParam = "target_connectivity_urls"
	n8nAuthTargetConnectivityURL   = "https://api.openai.com/v1/models"
)

type n8nActionJobConfig struct {
	Action                 string
	EngineParam            string
	AccountIDParam         string
	EmailParam             string
	TargetConnectivityURLs string
}

func newN8NActionJobID() string {
	return uuid.NewString()
}

func (cfg n8nActionJobConfig) withAction(profile contracts.ActionProfile) n8nActionJobConfig {
	cfg.Action = profile.ActionID
	return cfg
}

func (s *Server) createN8NActionJob(ctx context.Context, cfg n8nActionJobConfig, jobID string, accountID string, email string, params map[string]string) error {
	cfg = cfg.normalized()
	if cfg.Action == "" {
		return fmt.Errorf("n8n action job config is incomplete")
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return fmt.Errorf("job_id is required")
	}
	if params == nil {
		params = map[string]string{}
	}
	accountID = strings.TrimSpace(accountID)
	if accountID != "" {
		params[cfg.AccountIDParam] = accountID
	}
	params[cfg.EngineParam] = n8nEngineValue
	if cfg.EmailParam != "" {
		params[cfg.EmailParam] = emailx.Normalize(email)
	}
	if cfg.TargetConnectivityURLs != "" {
		params[n8nTargetConnectivityURLsParam] = cfg.TargetConnectivityURLs
	}
	_, err := s.jobStore.CreateWithID(ctx, jobID, accountID, cfg.Action, params)
	return err
}

func (cfg n8nActionJobConfig) normalized() n8nActionJobConfig {
	cfg.Action = strings.TrimSpace(cfg.Action)
	cfg.EngineParam = strings.TrimSpace(cfg.EngineParam)
	if cfg.EngineParam == "" {
		cfg.EngineParam = n8nDefaultEngineParam
	}
	cfg.AccountIDParam = strings.TrimSpace(cfg.AccountIDParam)
	if cfg.AccountIDParam == "" {
		cfg.AccountIDParam = n8nDefaultAccountIDParam
	}
	cfg.EmailParam = strings.TrimSpace(cfg.EmailParam)
	cfg.TargetConnectivityURLs = strings.TrimSpace(cfg.TargetConnectivityURLs)
	return cfg
}
