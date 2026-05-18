package manualinput

import (
	"context"
	"strconv"
	"strings"
)

type ParamStore interface {
	GetParam(context.Context, string, string) (string, bool, error)
	DeleteParam(context.Context, string, string) error
}

func OTPSubmittedAfter(ctx context.Context, store ParamStore, jobID, otpParam, submittedAtParam string, issuedAfterUnix int64) bool {
	return SubmittedAfter(ctx, store, jobID, otpParam, submittedAtParam, issuedAfterUnix)
}

func SubmittedAfter(ctx context.Context, store ParamStore, jobID, valueParam, submittedAtParam string, issuedAfterUnix int64) bool {
	if issuedAfterUnix <= 0 {
		return true
	}
	submittedAtValue, found, err := store.GetParam(ctx, jobID, submittedAtParam)
	if err != nil || !found {
		_ = store.DeleteParam(ctx, jobID, valueParam)
		_ = store.DeleteParam(ctx, jobID, submittedAtParam)
		return false
	}
	submittedAt, err := strconv.ParseInt(strings.TrimSpace(submittedAtValue), 10, 64)
	if err != nil || submittedAt < issuedAfterUnix {
		_ = store.DeleteParam(ctx, jobID, valueParam)
		_ = store.DeleteParam(ctx, jobID, submittedAtParam)
		return false
	}
	return true
}
