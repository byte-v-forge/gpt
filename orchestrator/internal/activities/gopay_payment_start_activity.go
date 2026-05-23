package activities

import (
	"context"
	"strings"
)

func (s *Server) GoPayPaymentStartActivity(ctx context.Context, input GoPayActivityInput) (GoPayPaymentStartOutput, error) {
	output := GoPayPaymentStartOutput{}
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepGoPayPayment, "starting gopay payment", goPayPaymentHeartbeatFields(input))
	defer stopHeartbeat()

	step, err := s.startActivityStep(ctx, input.GetJobId(), stepGoPayPayment, false, true)
	if err != nil {
		return output, err
	}
	account, err := s.paymentActivityAccount(ctx, &input)
	if err != nil {
		return output, s.completeGoPayPaymentStep(ctx, input.GetJobId(), input.GetAccountId(), map[string]any{"error_message": err.Error()}, err)
	}
	if account != nil && strings.TrimSpace(input.GetSessionToken()) == "" && strings.TrimSpace(input.GetAccessToken()) == "" {
		if err := accountEligibleForActivation(account); err != nil {
			return output, s.completeGoPayPaymentStep(ctx, input.GetJobId(), input.GetAccountId(), map[string]any{"account_id": account.GetAccountId(), "error_message": err.Error()}, err)
		}
	}

	output, err = s.startGoPayPayment(ctx, step, input, account)
	if err != nil {
		return output, s.completeGoPayPaymentStep(ctx, input.GetJobId(), input.GetAccountId(), protoDataMap(output.GetData()), err)
	}
	step.update(protoDataMap(output.GetData()))
	return output, nil
}
