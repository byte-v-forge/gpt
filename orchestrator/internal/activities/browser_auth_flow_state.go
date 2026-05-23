package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"orchestrator/pb"
)

func (f *browserAuthFlow) complete(ctx context.Context, otp string) (*pb.RegisterResponse, error) {
	code := normalizeOTP(otp)
	if code == "" {
		return &pb.RegisterResponse{Success: false, ErrorMessage: "otp is required"}, nil
	}
	select {
	case f.otpCh <- code:
	case <-f.doneCh:
		return f.registerResponse(), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-f.doneCh:
		return f.registerResponse(), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (f *browserAuthFlow) waitForOTP() (string, error) {
	now := time.Now().Unix()
	f.mu.Lock()
	f.otpNeed = true
	if f.otpAction <= 0 {
		f.otpAction = now
	}
	f.otpWait = now
	f.stage = browserAuthStageWaitingForOTP
	f.message = "waiting for orchestrator-supplied OTP"
	f.updatedAt = now
	f.mu.Unlock()

	select {
	case otp := <-f.otpCh:
		if otp == "" {
			return "", fmt.Errorf("OTP is empty")
		}
		return otp, nil
	case <-f.ctx.Done():
		return "", f.ctx.Err()
	}
}

func (f *browserAuthFlow) markOTPRequestClicked() {
	f.markOTPRequestClickedAt(time.Now().Unix())
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
		f.cancelFlow("browser auth cancelled")
		return
	}
	f.mu.Lock()
	f.success = false
	f.errMessage = message
	f.result = &pb.RegisterResponse{Success: false, ErrorMessage: message}
	f.done = true
	f.stage = browserAuthStageFailed
	f.message = message
	f.updatedAt = time.Now().Unix()
	f.mu.Unlock()
}

func (f *browserAuthFlow) cancelFlow(message string) {
	f.cancel()
	f.mu.Lock()
	f.success = false
	f.errMessage = message
	f.result = &pb.RegisterResponse{Success: false, ErrorMessage: message}
	f.done = true
	f.stage = browserAuthStageCancelled
	f.message = message
	f.updatedAt = time.Now().Unix()
	f.mu.Unlock()
	f.finish()
}

func (f *browserAuthFlow) finish() {
	f.doneOnce.Do(func() {
		close(f.doneCh)
	})
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
		FlowId:                        f.flowID,
		OtpRequired:                   f.otpNeed,
		OtpIssuedAfterUnix:            f.otpAction,
		Stage:                         f.stage,
		StatusMessage:                 f.message,
		OtpWaitStartedAtUnix:          f.otpWait,
		OtpRequestActionStartedAtUnix: f.otpAction,
		Result:                        cloneRegisterResult(f.result),
	}
}

func (f *browserAuthFlow) statusResponse() *pb.BrowserFlowStatusResponse {
	f.mu.Lock()
	defer f.mu.Unlock()
	resp := &pb.BrowserFlowStatusResponse{
		Found:                         true,
		FlowId:                        f.flowID,
		Mode:                          f.mode,
		Stage:                         f.stage,
		StatusMessage:                 f.message,
		OtpRequired:                   f.otpNeed,
		Done:                          f.done,
		Success:                       f.success,
		ErrorMessage:                  f.errMessage,
		StartedAtUnix:                 f.startedAt,
		UpdatedAtUnix:                 f.updatedAt,
		OtpIssuedAfterUnix:            f.otpAction,
		OtpWaitStartedAtUnix:          f.otpWait,
		OtpRequestActionStartedAtUnix: f.otpAction,
		Result:                        cloneRegisterResult(f.result),
	}
	return resp
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
