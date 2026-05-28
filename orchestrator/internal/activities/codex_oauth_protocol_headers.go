package activities

import (
	"fmt"

	"github.com/byte-v-forge/common-lib/randx"
)

func codexOAuthProtocolDatadogHeaders() map[string]string {
	traceID := codexOAuthProtocolRandomDecimalID()
	parentID := codexOAuthProtocolRandomDecimalID()
	traceHex := codexOAuthProtocolRandomHex(8)
	parentHex := codexOAuthProtocolRandomHex(8)
	return map[string]string{
		"traceparent":                 fmt.Sprintf("00-0000000000000000%s-%s-01", traceHex, parentHex),
		"tracestate":                  "dd=s:1;o:rum",
		"x-datadog-origin":            "rum",
		"x-datadog-parent-id":         parentID,
		"x-datadog-sampling-priority": "1",
		"x-datadog-trace-id":          traceID,
	}
}

func codexOAuthProtocolRandomDecimalID() string {
	n, err := randx.PositiveInt63()
	if err != nil || n <= 0 {
		return "1"
	}
	return fmt.Sprint(n)
}

func codexOAuthProtocolRandomHex(size int) string {
	value, err := randx.Hex(size)
	if err != nil {
		return "0000000000000001"
	}
	return value
}
