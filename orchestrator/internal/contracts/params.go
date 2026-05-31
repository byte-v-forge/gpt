package contracts

const (
	JobParamRegistrationOTP                      = "registration_otp"
	JobParamRegistrationOTPSubmittedAtUnix       = "registration_otp_submitted_at_unix"
	JobParamRegistrationOTPResendRequestedAtUnix = "registration_otp_resend_requested_at_unix"

	JobParamChannelOTPChannel         = "channel_otp_channel"
	JobParamChannelOTPTarget          = "channel_otp_target"
	JobParamChannelOTPIssuedAfter     = "channel_otp_issued_after"
	JobParamChannelOTPTimeoutSeconds  = "channel_otp_timeout_seconds"
	JobParamChannelOTPResumedAtUnix   = "channel_otp_resumed_at_unix"
	JobParamChannelOTP                = "otp"
	JobParamChannelOTPSubmittedAtUnix = "otp_submitted_at_unix"
)
