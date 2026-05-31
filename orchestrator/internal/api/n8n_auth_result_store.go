package api

import (
	"context"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/protojsonx"

	"orchestrator/pb"
)

func (s *Server) saveN8NAuthResult(ctx context.Context, resultSecretPrefix string, jobID string, result *pb.RegisterActivityOutput) (string, error) {
	if result == nil {
		return "", nil
	}
	key := strings.TrimSpace(resultSecretPrefix) + strings.TrimSpace(jobID)
	data, err := protojsonx.Marshal(result)
	if err != nil {
		return "", err
	}
	if err := s.saveRuntimeSecretValueTTL(ctx, key, string(data), s.runtimeSecretTTL()); err != nil {
		return "", err
	}
	return key, nil
}

func (s *Server) loadN8NAuthResult(ctx context.Context, resultSecretPrefix string, jobID string, resultRef string) (*pb.RegisterActivityOutput, error) {
	resultRef = strings.TrimSpace(resultRef)
	if resultRef == "" {
		resultRef = strings.TrimSpace(resultSecretPrefix) + strings.TrimSpace(jobID)
	}
	raw, err := s.runtimeSecretValue(ctx, resultRef)
	if err != nil {
		return nil, err
	}
	out := &pb.RegisterActivityOutput{}
	if err := protojsonx.Unmarshal([]byte(raw), out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Server) runtimeSecretTTL() time.Duration {
	if s == nil || s.runtimeSecrets == nil || s.runtimeSecrets.DefaultTTL() <= 0 {
		return time.Hour
	}
	return s.runtimeSecrets.DefaultTTL()
}
