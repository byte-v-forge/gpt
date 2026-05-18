package jobstatus

const (
	Created           = "CREATED"
	Running           = "RUNNING"
	Succeeded         = "SUCCEEDED"
	FailedRecoverable = "FAILED_RECOVERABLE"
	FailedRetryable   = "FAILED_RETRYABLE"
	FailedFinal       = "FAILED_FINAL"
)

func Failed(recoverable bool, retryable bool) string {
	if recoverable {
		return FailedRecoverable
	}
	if retryable {
		return FailedRetryable
	}
	return FailedFinal
}
