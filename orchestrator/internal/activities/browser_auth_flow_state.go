package activities

import (
	"context"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
	"orchestrator/pb"
)

func (f *browserAuthFlow) markWaitingForOTP(kind string, issuedAfterUnix int64) {
	now := time.Now().Unix()
	if issuedAfterUnix <= 0 {
		issuedAfterUnix = now
	}
	f.mu.Lock()
	f.otpNeed = true
	f.otpKind = kind
	f.otpAction = issuedAfterUnix
	f.otpWait = now
	f.stage = browserAuthStageWaitingForOTP
	f.message = "waiting for orchestrator-supplied OTP"
	f.updatedAt = now
	f.mu.Unlock()
}

func (f *browserAuthFlow) markOTPRequestClickedAt(issuedAfterUnix int64) {
	now := time.Now().Unix()
	if issuedAfterUnix <= 0 {
		issuedAfterUnix = now
	}
	f.mu.Lock()
	f.otpAction = issuedAfterUnix
	f.stage = browserAuthStageOTPRequestClicked
	f.message = "OTP request action clicked"
	f.updatedAt = now
	f.mu.Unlock()
}

func (f *browserAuthFlow) setStatus(stage, message string) {
	now := time.Now().Unix()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stage = stage
	f.message = message
	f.updatedAt = now
}

func (f *browserAuthFlow) fail(err error) {
	message := "browser auth failed"
	if err != nil {
		message = err.Error()
	}
	if errors.Is(err, context.Canceled) {
		message = "browser auth cancelled"
	}
	f.mu.Lock()
	f.success = false
	f.errMessage = message
	f.result = &pb.RegisterResponse{Success: false, ErrorMessage: message}
	f.done = true
	if errors.Is(err, context.Canceled) {
		f.stage = browserAuthStageCancelled
	} else {
		f.stage = browserAuthStageFailed
	}
	f.message = message
	f.updatedAt = time.Now().Unix()
	f.mu.Unlock()
}

func (f *browserAuthFlow) cancelled() bool {
	select {
	case <-f.ctx.Done():
		return true
	default:
		return false
	}
}

func (f *browserAuthFlow) getSessionID() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sessionID
}

func (f *browserAuthFlow) startResponse() *pb.StartRegisterResponse {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &pb.StartRegisterResponse{
		Success:                       true,
		BrowserSessionId:              f.sessionID,
		OtpRequired:                   f.otpNeed,
		OtpIssuedAfterUnix:            f.otpAction,
		Stage:                         f.stage,
		StatusMessage:                 f.message,
		OtpWaitStartedAtUnix:          f.otpWait,
		OtpRequestActionStartedAtUnix: f.otpAction,
		Result:                        cloneRegisterResult(f.result),
	}
}

func (f *browserAuthFlow) registerResponse() *pb.RegisterResponse {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.result != nil {
		return cloneRegisterResult(f.result)
	}
	if f.done && f.errMessage != "" {
		return &pb.RegisterResponse{Success: false, ErrorMessage: f.errMessage}
	}
	return &pb.RegisterResponse{Success: false, ErrorMessage: "browser flow did not complete"}
}

func cloneRegisterResult(in *pb.RegisterResponse) *pb.RegisterResponse {
	if in == nil {
		return nil
	}
	return proto.Clone(in).(*pb.RegisterResponse)
}
