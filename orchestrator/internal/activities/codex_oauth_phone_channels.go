package activities

import (
	"fmt"
	"strings"
)

var codexOAuthSMSCountryNames = map[string]string{
	"CA": "Canada",
	"FK": "Falkland Islands",
	"FR": "France",
	"JP": "Japan",
	"KR": "South Korea",
	"NU": "Niue",
	"SM": "San Marino",
	"TH": "Thailand",
	"TL": "Timor-Leste",
	"TW": "Taiwan",
	"US": "United States",
	"VU": "Vanuatu",
}

func validateCodexOAuthSMSCountry(countryISO2 string) error {
	code := strings.ToUpper(strings.TrimSpace(countryISO2))
	if _, ok := codexOAuthSMSCountryNames[code]; ok {
		return nil
	}
	return fmt.Errorf("codex oauth sms is not supported for country %q", code)
}

func codexOAuthPhoneCountryLabels(phone *CodexOAuthPhoneLease) []string {
	code := strings.ToUpper(strings.TrimSpace(phone.GetCountryIso2()))
	name := codexOAuthSMSCountryNames[code]
	callingCode := strings.TrimPrefix(strings.TrimSpace(phone.GetCountryCallingCode()), "+")
	if name == "" {
		return nil
	}
	if callingCode == "" {
		return []string{name}
	}
	return []string{name + " (+" + callingCode + ")", name}
}
