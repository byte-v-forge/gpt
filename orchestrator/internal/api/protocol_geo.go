package api

import (
	"context"
	"strings"

	"orchestrator/pb"
)

const (
	protocolCountryCodeParam = "country_code"
	protocolRegionParam      = "region"
	protocolProxyURLParam    = "protocol_proxy_url"
)

func putProtocolGeoParams(params map[string]string, countryCode string, region string) {
	if params == nil {
		return
	}
	if value := normalizeProtocolCountryCode(countryCode); value != "" {
		params[protocolCountryCodeParam] = value
	}
	if value := normalizeProtocolRegion(countryCode, region); value != "" {
		params[protocolRegionParam] = value
	}
}

func (s *Server) protocolAuthStartInput(ctx context.Context, jobID string, accountID string, mode string) *pb.ProtocolAuthStartInput {
	params := map[string]string{}
	if s.jobStore != nil {
		params, _ = s.jobStore.Params(ctx, strings.TrimSpace(jobID))
	}
	return &pb.ProtocolAuthStartInput{
		JobId:       strings.TrimSpace(jobID),
		AccountId:   strings.TrimSpace(accountID),
		Mode:        strings.TrimSpace(mode),
		CountryCode: params[protocolCountryCodeParam],
		Region:      params[protocolRegionParam],
		ProxyUrl:    strings.TrimSpace(params[protocolProxyURLParam]),
	}
}

func (s *Server) protocolProxyURL(ctx context.Context, jobID string) string {
	if s.jobStore == nil {
		return ""
	}
	params, _ := s.jobStore.Params(ctx, strings.TrimSpace(jobID))
	return strings.TrimSpace(params[protocolProxyURLParam])
}

func normalizeProtocolCountryCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) > 2 {
		return value[:2]
	}
	return value
}

func normalizeProtocolRegion(countryCode string, value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" || strings.Contains(value, "-") {
		return value
	}
	countryCode = normalizeProtocolCountryCode(countryCode)
	if countryCode == "" {
		return value
	}
	return countryCode + "-" + value
}
