package paymentsvc

import "github.com/byte-v-forge/gpt/gopay/otpchannel"

func normalizeOTPChannel(value string) string {
	return otpchannel.Normalize(value)
}

func goPayOTPChannelRequiresSMSActivation(channel string) bool {
	return otpchannel.RequiresSMSActivation(channel)
}
