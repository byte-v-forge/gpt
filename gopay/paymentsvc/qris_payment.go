package paymentsvc

import (
	"context"
	"fmt"
	"time"
)

func (c *charger) startPreparedQRISToPaymentCharge(ctx context.Context, state map[string]any) (map[string]any, error) {
	snapToken := stringAt(state, "snap_token")
	if snapToken == "" {
		return nil, fmt.Errorf("prepared payment is missing snap_token")
	}
	if checkoutURL := stringAt(state, "checkout_url"); checkoutURL != "" {
		c.checkoutURL = checkoutURL
	}
	if err := c.midtransLoadTransaction(ctx, snapToken); err != nil {
		return nil, err
	}
	chargeData, err := c.midtransCreateChargeData(ctx, snapToken)
	if err != nil {
		return nil, err
	}
	next := cloneMap(state)
	for key, value := range chargeData {
		next[key] = value
	}
	next["state"] = "awaiting_manual_confirmation"
	next["issued_after_unix"] = time.Now().Unix()
	next["otp_required"] = false
	return next, nil
}
