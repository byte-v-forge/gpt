package api

import (
	"context"
	"strings"

	"orchestrator/internal/accountfingerprint"
	"orchestrator/pb"
)

func n8nDynamicProxyGeoData(accountID string, n8nExecutionID string, countryCode string, region string, geoSource string) *pb.N8NDynamicProxyGeoData {
	scope := n8nActionScopeFrom("", accountID, n8nExecutionID)
	return &pb.N8NDynamicProxyGeoData{
		AccountId:      strings.TrimSpace(scope.AccountID),
		N8NExecutionId: strings.TrimSpace(scope.N8NExecutionID),
		CountryCode:    strings.TrimSpace(countryCode),
		Region:         strings.TrimSpace(region),
		GeoSource:      strings.TrimSpace(geoSource),
	}
}

func (s *Server) dynamicProxyGeoFromFingerprint(ctx context.Context, accountID string, countryCode string, region string) (string, string, string) {
	source := "account_fingerprint"
	if s.fingerprints == nil || strings.TrimSpace(accountID) == "" {
		return countryCode, region, "missing"
	}
	profile, ok, err := s.fingerprints.Get(ctx, accountID)
	if err != nil || !ok {
		return countryCode, region, "missing"
	}
	if countryCode == "" {
		countryCode = normalizeProtocolCountryCode(profile.CountryCode)
	}
	if region == "" {
		region = normalizeProtocolRegion(countryCode, profile.Region)
	}
	if countryCode == "" || region == "" {
		inferredCountry, inferredRegion, ok := accountfingerprint.GeoFromTimezone(profile.Timezone)
		if ok {
			source = "account_fingerprint_timezone"
			if countryCode == "" {
				countryCode = normalizeProtocolCountryCode(inferredCountry)
			}
			if region == "" {
				region = normalizeProtocolRegion(countryCode, inferredRegion)
			}
		}
	}
	return countryCode, region, source
}

func protocolProxyState(countryCode string, region string) string {
	countryCode = normalizeProtocolCountryCode(countryCode)
	region = normalizeProtocolRegion(countryCode, region)
	prefix, state, ok := strings.Cut(region, "-")
	if !ok || state == "" {
		return ""
	}
	if countryCode != "" && !strings.EqualFold(prefix, countryCode) {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(state))
}
