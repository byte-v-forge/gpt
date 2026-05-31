package activities

import (
	"log"
	"strings"

	"orchestrator/pb"
)

type codexOAuthProtocolProxyGeo struct {
	CountryCode string
	Region      string
	State       string
}

func codexOAuthProtocolProxyGeoFromInput(countryCode string, region string) codexOAuthProtocolProxyGeo {
	countryCode = normalizeCodexOAuthProtocolProxyCountry(countryCode)
	region = normalizeCodexOAuthProtocolProxyRegion(countryCode, region)
	if countryCode == "" && strings.Contains(region, "-") {
		countryCode, _, _ = strings.Cut(region, "-")
		countryCode = normalizeCodexOAuthProtocolProxyCountry(countryCode)
	}
	return codexOAuthProtocolProxyGeo{
		CountryCode: countryCode,
		Region:      region,
		State:       codexOAuthProtocolProxyState(countryCode, region),
	}
}

func recordCodexOAuthProtocolProxyRequestGeo(data *pb.ActivityProtocolProxyUseData, geo codexOAuthProtocolProxyGeo) {
	if data == nil {
		return
	}
	if geo.CountryCode != "" {
		data.ProtocolProxyRequestedCountry = geo.CountryCode
	}
	if geo.Region != "" {
		data.ProtocolProxyRequestedRegion = geo.Region
	}
	if geo.State != "" {
		data.ProtocolProxyRequestedState = geo.State
	}
}

func normalizeCodexOAuthProtocolProxyCountry(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) > 2 {
		return value[:2]
	}
	return value
}

func normalizeCodexOAuthProtocolProxyRegion(countryCode string, value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" || strings.Contains(value, "-") {
		return value
	}
	if countryCode = normalizeCodexOAuthProtocolProxyCountry(countryCode); countryCode != "" {
		return countryCode + "-" + value
	}
	return value
}

func codexOAuthProtocolProxyState(countryCode string, region string) string {
	countryCode = normalizeCodexOAuthProtocolProxyCountry(countryCode)
	region = normalizeCodexOAuthProtocolProxyRegion(countryCode, region)
	prefix, state, ok := strings.Cut(region, "-")
	if !ok || state == "" {
		return ""
	}
	if countryCode != "" && !strings.EqualFold(prefix, countryCode) {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(state))
}

func logCodexOAuthProtocolProxyUse(accountID string, mode string, source string) {
	log.Printf(
		"[protocol-proxy] use proxy account_id=%s mode=%s source=%s",
		strings.TrimSpace(accountID),
		strings.TrimSpace(mode),
		strings.TrimSpace(source),
	)
}
