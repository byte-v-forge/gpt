package smsotp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	commonv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/common/v1"
	smsv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/sms/v1"
)

const defaultResolveTimeout = 10 * time.Second

type Resolver interface {
	ResolveCode(ctx context.Context, orderID string, ref *commonv1.SecretRef) (string, error)
}

type ClientResolver struct {
	client  smsv1.SmsOrderServiceClient
	timeout time.Duration
}

type ResolveError struct {
	Code      smsv1.SmsErrorCode
	Message   string
	Retryable bool
}

func NewClientResolver(client smsv1.SmsOrderServiceClient, timeout time.Duration) *ClientResolver {
	if timeout <= 0 {
		timeout = defaultResolveTimeout
	}
	return &ClientResolver{client: client, timeout: timeout}
}

func (r *ClientResolver) ResolveCode(ctx context.Context, orderID string, ref *commonv1.SecretRef) (string, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return "", &ResolveError{Code: smsv1.SmsErrorCode_SMS_ERROR_CODE_VALIDATION_FAILED, Message: "order_id is required"}
	}
	if ref == nil || strings.TrimSpace(ref.GetSecretId()) == "" {
		return "", &ResolveError{Code: smsv1.SmsErrorCode_SMS_ERROR_CODE_VALIDATION_FAILED, Message: "sms code secret ref is required"}
	}
	if r == nil || r.client == nil {
		return "", errors.New("sms code resolver client is required")
	}
	callCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	resp, err := r.client.ResolveSmsCodeSecret(callCtx, &smsv1.ResolveSmsCodeSecretRequest{
		OrderId:   orderID,
		SecretRef: ref,
	})
	if err != nil {
		return "", fmt.Errorf("resolve sms code secret: %w", err)
	}
	if err := resolveResponseError(resp.GetError()); err != nil {
		return "", err
	}
	code := strings.TrimSpace(resp.GetCodeValue())
	if code == "" {
		return "", &ResolveError{Code: smsv1.SmsErrorCode_SMS_ERROR_CODE_ORDER_EXPIRED, Message: "sms code secret resolved empty"}
	}
	return code, nil
}

func Retryable(err error) bool {
	var resolveErr *ResolveError
	if errors.As(err, &resolveErr) {
		return resolveErr.Retryable
	}
	return true
}

func (e *ResolveError) Error() string {
	if e == nil {
		return ""
	}
	code := e.Code.String()
	if strings.TrimSpace(e.Message) == "" {
		return "sms code resolve failed: " + code
	}
	return "sms code resolve failed: " + code + ": " + e.Message
}

func resolveResponseError(smsErr *smsv1.SmsError) error {
	if smsErr == nil {
		return nil
	}
	return &ResolveError{
		Code:      smsErr.GetCode(),
		Message:   strings.TrimSpace(smsErr.GetMessage()),
		Retryable: smsErr.GetRetryable(),
	}
}
