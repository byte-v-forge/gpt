package activities

import (
	"crypto/rand"
	"fmt"
	"math/big"
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
	max := new(big.Int).Lsh(big.NewInt(1), 63)
	n, err := rand.Int(rand.Reader, max)
	if err != nil || n.Sign() <= 0 {
		return "1"
	}
	return n.String()
}

func codexOAuthProtocolRandomHex(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "0000000000000001"
	}
	return fmt.Sprintf("%x", buf)
}
