package accountfingerprint

import "orchestrator/pb"

func Response(profile Profile, generated bool) *pb.AccountFingerprintResponse {
	return &pb.AccountFingerprintResponse{
		AccountId:              profile.AccountID,
		Generated:              generated,
		CountryCode:            profile.CountryCode,
		Region:                 profile.Region,
		BrowserProfileTemplate: profile.BrowserProfileTemplate,
		BrowserFamily:          profile.BrowserFamily,
		BrowserMajorVersion:    profile.BrowserMajorVersion,
		OsFamily:               profile.OSFamily,
		TlsProfileFamily:       profile.TLSProfileFamily,
		TlsFingerprintVariant:  profile.TLSFingerprintVariant,
		Locale:                 profile.Locale,
		Timezone:               profile.Timezone,
		UserAgent:              profile.UserAgent,
		AcceptLanguage:         profile.AcceptLanguage,
		Language:               profile.Language,
		DeviceId:               profile.DeviceID,
		CreatedAt:              profile.CreatedAt,
		UpdatedAt:              profile.UpdatedAt,
	}
}
