package activities

import (
	"context"
	"strings"
)

func (s *Server) GoPayPaymentPrepareActivity(ctx context.Context, input GoPayActivityInput) (GoPayPaymentPrepareOutput, error) {
	output := GoPayPaymentPrepareOutput{}
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepGoPayPaymentPrepare, "preparing gopay payment", goPayPaymentHeartbeatFields(input))
	defer stopHeartbeat()

	step, err := s.startActivityStep(ctx, input.GetJobId(), stepGoPayPaymentPrepare, false, true)
	if err != nil {
		return output, err
	}
	account, err := s.paymentActivityAccount(ctx, &input)
	if err != nil {
		return output, step.complete(map[string]any{"error_message": err.Error()}, err)
	}
	if account != nil && strings.TrimSpace(input.GetSessionToken()) == "" && strings.TrimSpace(input.GetAccessToken()) == "" {
		if err := accountEligibleForActivation(account); err != nil {
			return output, step.complete(map[string]any{"account_id": account.GetAccountId(), "error_message": err.Error()}, err)
		}
	}

	output, err = s.prepareGoPayPayment(ctx, step, input, account)
	if err != nil {
		return output, step.complete(protoDataMap(output.GetData()), err)
	}
	return output, step.complete(protoDataMap(output.GetData()), nil)
}

func (s *Server) GoPayPaymentPrepareCheckoutActivity(ctx context.Context, input GoPayActivityInput) (GoPayPaymentPrepareOutput, error) {
	output := GoPayPaymentPrepareOutput{}
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepGoPayPaymentPrepareCheckout, "creating gopay payment checkout", goPayPaymentHeartbeatFields(input))
	defer stopHeartbeat()

	step, err := s.startActivityStep(ctx, input.GetJobId(), stepGoPayPaymentPrepareCheckout, false, true)
	if err != nil {
		return output, err
	}
	account, err := s.paymentActivityAccount(ctx, &input)
	if err != nil {
		return output, step.complete(map[string]any{"error_message": err.Error()}, err)
	}
	if account != nil && strings.TrimSpace(input.GetSessionToken()) == "" && strings.TrimSpace(input.GetAccessToken()) == "" {
		if err := accountEligibleForActivation(account); err != nil {
			return output, step.complete(map[string]any{"account_id": account.GetAccountId(), "error_message": err.Error()}, err)
		}
	}

	output, err = s.prepareGoPayPaymentCheckout(ctx, step, input, account)
	if err != nil {
		return output, step.complete(protoDataMap(output.GetData()), err)
	}
	return output, step.complete(protoDataMap(output.GetData()), nil)
}

func (s *Server) GoPayPaymentPrepareRefreshActivity(ctx context.Context, input GoPayActivityInput) (GoPayPaymentPrepareOutput, error) {
	output := GoPayPaymentPrepareOutput{}
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepGoPayPaymentPrepareRefresh, "refreshing gopay payment checkout", goPayPaymentHeartbeatFields(input))
	defer stopHeartbeat()

	step, err := s.startActivityStep(ctx, input.GetJobId(), stepGoPayPaymentPrepareRefresh, false, true)
	if err != nil {
		return output, err
	}
	output, err = s.refreshGoPayPaymentCheckout(ctx, step, input)
	if err != nil {
		return output, step.complete(protoDataMap(output.GetData()), err)
	}
	return output, step.complete(protoDataMap(output.GetData()), nil)
}

func (s *Server) GoPayPaymentPrepareLinkActivity(ctx context.Context, input GoPayActivityInput) (GoPayPaymentPrepareOutput, error) {
	output := GoPayPaymentPrepareOutput{}
	stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepGoPayPaymentPrepareLink, "linking gopay payment checkout", goPayPaymentHeartbeatFields(input))
	defer stopHeartbeat()

	step, err := s.startActivityStep(ctx, input.GetJobId(), stepGoPayPaymentPrepareLink, false, true)
	if err != nil {
		return output, err
	}
	output, err = s.prepareGoPayPaymentLink(ctx, step, input)
	if err != nil {
		return output, step.complete(protoDataMap(output.GetData()), err)
	}
	return output, step.complete(protoDataMap(output.GetData()), nil)
}
