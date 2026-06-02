package api

import (
	"context"
	"strings"

	"github.com/byte-v-forge/common-lib/emailx"

	"orchestrator/pb"
)

type protocolAuthProgress interface {
	GetAccountId() string
	GetFlowId() string
	GetEmail() string
	GetOtpRequired() bool
	GetOtpIssuedAfterUnix() int64
	GetOtpTimeoutSeconds() int32
	GetResult() *pb.RegisterActivityOutput
}

type registerOutputProgress struct {
	accountID string
	flowID    string
	result    *pb.RegisterActivityOutput
}

func (p *registerOutputProgress) GetAccountId() string                  { return strings.TrimSpace(p.accountID) }
func (p *registerOutputProgress) GetFlowId() string                     { return strings.TrimSpace(p.flowID) }
func (p *registerOutputProgress) GetEmail() string                      { return "" }
func (p *registerOutputProgress) GetOtpRequired() bool                  { return false }
func (p *registerOutputProgress) GetOtpIssuedAfterUnix() int64          { return 0 }
func (p *registerOutputProgress) GetOtpTimeoutSeconds() int32           { return 0 }
func (p *registerOutputProgress) GetResult() *pb.RegisterActivityOutput { return p.result }

func n8nAuthStepResultFromBase(base *pb.N8NActionStepResult) *pb.N8NAuthStepResult {
	return &pb.N8NAuthStepResult{
		JobId:          base.GetJobId(),
		AccountId:      base.GetAccountId(),
		N8NExecutionId: base.GetN8NExecutionId(),
		Step:           base.GetStep(),
		Success:        base.GetSuccess(),
	}
}

func (s *Server) n8nBrowserAuthStartResult(ctx context.Context, resultSecretPrefix string, jobID string, accountID string, n8nExecutionID string, step string, out *pb.BrowserAuthStartOutput) *pb.N8NAuthStepResult {
	if out == nil {
		return n8nAuthStepResultFromBase(n8nActionStep(jobID, accountID, n8nExecutionID, step, true))
	}
	flowID := strings.TrimSpace(out.GetBrowserSessionId())
	result := n8nAuthStepResultFromBase(n8nActionStep(jobID, accountID, n8nExecutionID, step, true))
	result.FlowId = flowID
	result.Email = emailx.Normalize(out.GetEmail())
	result.OtpRequired = out.GetOtpRequired()
	result.OtpIssuedAfterUnix = out.GetOtpIssuedAfterUnix()
	result.OtpTimeoutSeconds = out.GetOtpTimeoutSeconds()
	if register := out.GetResult(); register != nil {
		if ref, err := s.saveN8NAuthResult(ctx, resultSecretPrefix, jobID, register); err == nil {
			result.ResultReady = true
			result.ResultRef = ref
		} else {
			result.ResultSecretError = err.Error()
		}
	}
	return result
}

func (s *Server) n8nBrowserAuthOutputResult(ctx context.Context, resultSecretPrefix string, jobID string, accountID string, n8nExecutionID string, step string, flowID string, out *pb.RegisterActivityOutput) (*pb.N8NAuthStepResult, error) {
	result := n8nAuthStepResultFromBase(n8nActionStep(jobID, accountID, n8nExecutionID, step, true))
	result.FlowId = strings.TrimSpace(flowID)
	if out != nil {
		ref, err := s.saveN8NAuthResult(ctx, resultSecretPrefix, jobID, out)
		if err != nil {
			return result, err
		}
		result.ResultReady = true
		result.ResultRef = ref
	}
	return result, nil
}

func n8nAuthStepResultFromChannelOTP(out *pb.N8NChannelOTPWaitResult) *pb.N8NAuthStepResult {
	if out == nil {
		return n8nAuthStepResultFromBase(n8nActionStep("", "", "", "", true))
	}
	result := n8nAuthStepResultFromBase(n8nActionStep(out.GetJobId(), out.GetAccountId(), out.GetN8NExecutionId(), out.GetStep(), true))
	result.FlowId = strings.TrimSpace(out.GetFlowId())
	result.Channel = strings.TrimSpace(out.GetChannel())
	result.Target = strings.TrimSpace(out.GetTarget())
	result.OtpRequired = true
	result.OtpFound = out.GetOtpFound()
	result.OtpIssuedAfterUnix = out.GetOtpIssuedAfterUnix()
	result.OtpTimeoutSeconds = out.GetOtpTimeoutSeconds()
	result.OtpSource = strings.TrimSpace(out.GetOtpSource())
	result.MessageId = strings.TrimSpace(out.GetMessageId())
	result.Waiting = out.GetWaiting()
	return result
}

func (s *Server) n8nProtocolAuthProgressResult(ctx context.Context, resultSecretPrefix string, jobID string, accountID string, n8nExecutionID string, step string, progress protocolAuthProgress) (*pb.N8NAuthStepResult, error) {
	result := n8nAuthStepResultFromBase(n8nActionStep(jobID, accountID, n8nExecutionID, step, true))
	if progress == nil {
		return result, nil
	}
	result.FlowId = strings.TrimSpace(progress.GetFlowId())
	result.Email = emailx.Normalize(progress.GetEmail())
	result.OtpRequired = progress.GetOtpRequired()
	result.OtpIssuedAfterUnix = progress.GetOtpIssuedAfterUnix()
	result.OtpTimeoutSeconds = progress.GetOtpTimeoutSeconds()
	if result.GetAccountId() == "" {
		result.AccountId = strings.TrimSpace(progress.GetAccountId())
	}
	if register := progress.GetResult(); register != nil {
		ref, err := s.saveN8NAuthResult(ctx, resultSecretPrefix, jobID, register)
		if err != nil {
			return result, err
		}
		result.ResultReady = true
		result.ResultRef = ref
	}
	return result, nil
}

func n8nProtocolStepResult(jobID string, accountID string, n8nExecutionID string, step string) *pb.N8NAuthStepResult {
	return n8nAuthStepResultFromBase(n8nActionStep(jobID, accountID, n8nExecutionID, step, false))
}
