package paymentsvc

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

var checkoutAmountKeys = map[string]bool{
	"due":              true,
	"amount_total":     true,
	"amount_due":       true,
	"total_amount":     true,
	"amount_remaining": true,
	"total":            true,
}

var checkoutAmountExcluded = map[string]bool{
	"amount_discount": true,
	"amount_subtotal": true,
	"amount_tax":      true,
	"discount":        true,
	"discounts":       true,
	"display_items":   true,
	"items":           true,
	"line_items":      true,
	"lines":           true,
	"price":           true,
	"prices":          true,
	"subtotal":        true,
	"tax":             true,
	"taxes":           true,
	"unit_amount":     true,
}

type amountCandidate struct {
	source string
	amount int64
}

func (c *charger) probePlusTrialCheckout(ctx context.Context) (trialProbe, error) {
	csID, err := c.createCheckout(ctx)
	if err != nil {
		return trialProbe{}, fmt.Errorf("checkout create failed: %w", err)
	}
	checkoutURL := firstNonEmpty(c.checkoutURL, "https://checkout.stripe.com/c/pay/"+csID)
	initData, err := c.stripeInit(ctx, csID)
	if err != nil {
		return trialProbe{
			CheckoutSessionID: csID,
			CheckoutURL:       checkoutURL,
			Checked:           false,
			PlusTrialEligible: false,
			Source:            "stripe_init_error",
			ErrorMessage:      "stripe init failed: " + truncateString(err.Error(), 500),
		}, nil
	}
	amount, source := selectCheckoutAmount(initData)
	checked := source != ""
	return trialProbe{
		CheckoutSessionID: csID,
		CheckoutURL:       checkoutURL,
		Checked:           checked,
		PlusTrialEligible: checked && amount == 0,
		Amount:            amount,
		Currency:          strings.ToUpper(stringAt(initData, "currency")),
		Source:            source,
		ErrorMessage:      mapBoolString(checked, "", "stripe init did not expose checkout amount"),
	}, nil
}

func (c *charger) resolveExpectedAmount(initData map[string]any) (string, string, error) {
	if override := strings.TrimSpace(c.cfg.Runtime["expected_amount"]); override != "" {
		amount, ok := parseStripeAmount(override)
		if !ok {
			return "", "", fmt.Errorf("invalid runtime expected amount: %q", override)
		}
		return strconv.FormatInt(amount, 10), "runtime.expected_amount", nil
	}
	amount, source := selectCheckoutAmount(initData)
	if source == "" {
		if configBool(c.cfg.Runtime["fail_on_unknown_expected_amount"]) {
			return "", "", fmt.Errorf("stripe init did not expose checkout amount; refusing confirm")
		}
		return "0", "fallback_zero_unknown", nil
	}
	if amount != 0 && !configBool(c.cfg.Runtime["allow_nonzero_expected_amount"]) {
		currency := firstNonEmpty(strings.ToUpper(stringAt(initData, "currency")), "UNKNOWN")
		return "", "", fmt.Errorf("checkout amount is %d %s from %s, not free-trial 0; refusing to confirm payment", amount, currency, source)
	}
	return strconv.FormatInt(amount, 10), source, nil
}

func selectCheckoutAmount(value map[string]any) (int64, string) {
	candidates := amountCandidates(value, nil)
	if len(candidates) == 0 {
		return 0, ""
	}
	preferredKeys := []string{"due", "amount_total", "amount_due", "total_amount", "amount_remaining", "total"}
	preferredContexts := []string{"total_summary", "checkout", "session", "invoice", "subscription"}
	for _, key := range preferredKeys {
		for _, candidate := range candidates {
			parts := strings.Split(strings.ToLower(candidate.source), ".")
			if len(parts) == 0 || parts[len(parts)-1] != key {
				continue
			}
			if len(parts) == 1 || pathHasAny(parts, preferredContexts) {
				return candidate.amount, candidate.source
			}
		}
	}
	return candidates[0].amount, candidates[0].source
}

func amountCandidates(value any, path []string) []amountCandidate {
	var out []amountCandidate
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := append(append([]string{}, path...), key)
			lower := strings.ToLower(key)
			if checkoutAmountKeys[lower] && !pathHasExcluded(childPath) {
				if amount, ok := parseStripeAmount(child); ok {
					out = append(out, amountCandidate{source: strings.Join(childPath, "."), amount: amount})
				}
			}
			out = append(out, amountCandidates(child, childPath)...)
		}
	case []any:
		for _, child := range typed {
			out = append(out, amountCandidates(child, path)...)
		}
	}
	return out
}

func parseStripeAmount(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), typed >= 0
	case int64:
		return typed, typed >= 0
	case float64:
		if typed < 0 {
			return 0, false
		}
		return int64(typed), true
	case string:
		if strings.TrimSpace(typed) == "" {
			return 0, false
		}
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil && parsed >= 0
	default:
		return 0, false
	}
}

func pathHasExcluded(path []string) bool {
	for _, part := range path {
		if checkoutAmountExcluded[strings.ToLower(part)] {
			return true
		}
	}
	return false
}

func pathHasAny(path []string, values []string) bool {
	for _, part := range path {
		for _, value := range values {
			if part == value {
				return true
			}
		}
	}
	return false
}

func configBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func mapBoolString(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

func truncateString(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}
