package activities

import smsv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/sms/v1"

func smsCancelSettled(resp *smsv1.CancelOrderResponse) bool {
	if resp == nil {
		return false
	}
	if resp.GetError() == nil {
		return true
	}
	switch resp.GetError().GetCode() {
	case smsv1.SmsErrorCode_SMS_ERROR_CODE_ORDER_NOT_FOUND,
		smsv1.SmsErrorCode_SMS_ERROR_CODE_ORDER_ALREADY_FINALIZED,
		smsv1.SmsErrorCode_SMS_ERROR_CODE_ORDER_EXPIRED:
		return true
	default:
		return false
	}
}

func smsCancelResponseText(resp *smsv1.CancelOrderResponse) string {
	if resp == nil {
		return "empty response"
	}
	if resp.GetError() != nil {
		return smsErrorText(resp.GetError())
	}
	return "unknown error"
}
