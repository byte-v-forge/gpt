package otpchannel

import (
	"strings"

	"github.com/byte-v-forge/gpt/gopay/pb"
)

func Normalize(value string) string {
	descriptor := Descriptor(value)
	if descriptor == nil {
		return ""
	}
	return descriptor.GetApiValue()
}

func ProviderMethod(value string) string {
	descriptor := Descriptor(value)
	if descriptor == nil {
		return ""
	}
	return descriptor.GetProviderMethod()
}

func RequiresSMSActivation(value string) bool {
	descriptor := Descriptor(value)
	return descriptor != nil && descriptor.GetRequiresSmsActivation()
}

func Descriptor(value string) *pb.GoPayOTPChannelDescriptor {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return nil
	}
	for _, descriptor := range Descriptors() {
		if normalized == descriptor.GetApiValue() || normalized == descriptor.GetProviderMethod() {
			return descriptor
		}
		for _, alias := range descriptor.GetAliases() {
			if normalized == strings.ToLower(strings.TrimSpace(alias)) {
				return descriptor
			}
		}
	}
	return nil
}

func Descriptors() []*pb.GoPayOTPChannelDescriptor {
	return []*pb.GoPayOTPChannelDescriptor{
		{
			Channel:               pb.GoPayOTPChannel_GOPAY_OTP_CHANNEL_SMS,
			ApiValue:              "sms",
			ProviderMethod:        "otp_sms",
			Aliases:               []string{"sms", "otp_sms"},
			RequiresSmsActivation: true,
		},
		{
			Channel:        pb.GoPayOTPChannel_GOPAY_OTP_CHANNEL_WHATSAPP,
			ApiValue:       "wa",
			ProviderMethod: "otp_wa",
			Aliases:        []string{"wa", "whatsapp", "otp_wa"},
		},
	}
}

func DefaultProviderMethod() string {
	return Descriptors()[0].GetProviderMethod()
}

func ProviderMethodFallbacks(defaultMethod string) []string {
	defaultMethod = ProviderMethod(defaultMethod)
	if defaultMethod == "" {
		defaultMethod = DefaultProviderMethod()
	}
	fallbacks := []string{defaultMethod}
	for _, descriptor := range Descriptors() {
		method := descriptor.GetProviderMethod()
		if method != "" && method != defaultMethod {
			fallbacks = append(fallbacks, method)
		}
	}
	return fallbacks
}
