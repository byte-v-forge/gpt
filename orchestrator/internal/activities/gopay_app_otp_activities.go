package activities

import (
	"context"
	"fmt"
)

const (
	goPayAppOTPOperationAuth   = "auth"
	goPayAppOTPOperationSignup = "signup"
)

func (s *Server) GoPayAppOTPStartActivity(ctx context.Context, input GoPayAppOTPStartInput) (GoPayAppOTPOutput, error) {
	switch input.GetOperation() {
	case goPayAppOTPOperationAuth:
		return s.startGoPayAppAuth(ctx, input)
	case goPayAppOTPOperationSignup:
		return s.startGoPayAppSignup(ctx, input)
	default:
		return GoPayAppOTPOutput{}, fmt.Errorf("unsupported gopay app otp operation: %s", input.GetOperation())
	}
}

func (s *Server) GoPayAppOTPCompleteActivity(ctx context.Context, input GoPayAppOTPCompleteInput) (GoPayAppOTPOutput, error) {
	switch input.GetOperation() {
	case goPayAppOTPOperationAuth:
		return s.completeGoPayAppAuth(ctx, input)
	case goPayAppOTPOperationSignup:
		return s.completeGoPayAppSignup(ctx, input)
	default:
		return GoPayAppOTPOutput{}, fmt.Errorf("unsupported gopay app otp operation: %s", input.GetOperation())
	}
}

func (s *Server) GoPayAppOTPRetryActivity(ctx context.Context, input GoPayAppOTPStartInput) (GoPayAppOTPOutput, error) {
	switch input.GetOperation() {
	case goPayAppOTPOperationSignup:
		return s.retryGoPayAppSignupOTP(ctx, input)
	default:
		return GoPayAppOTPOutput{}, fmt.Errorf("unsupported gopay app otp retry operation: %s", input.GetOperation())
	}
}
