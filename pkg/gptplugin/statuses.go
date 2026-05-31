package gptplugin

const (
	AccountStatusRegistered         = "REGISTERED"
	AccountStatusUnregistered       = "UNREGISTERED"
	AccountStatusActivated          = "ACTIVATED"
	AccountStatusDeactivated        = "DEACTIVATED"
	AccountStatusUserAlreadyExists  = "USER_ALREADY_EXISTS"
	AccountStatusEmailAlreadyExists = "EMAIL_ALREADY_EXISTS"
)

const (
	EmailStatusAvailable         = "AVAILABLE"
	EmailStatusAssigned          = "ASSIGNED"
	EmailStatusRegistered        = "REGISTERED"
	EmailStatusOAuthPending      = "OAUTH_PENDING"
	EmailStatusAuthFailed        = "AUTH_FAILED"
	EmailStatusUserAlreadyExists = "USER_ALREADY_EXISTS"
	EmailStatusBlocked           = "BLOCKED"
)
