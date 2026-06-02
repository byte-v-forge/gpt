package activities

import (
	"context"
	"orchestrator/internal/contracts"
)

func (s *Server) CodexOAuthAddPhoneBrowserActivity(ctx context.Context, input *CodexOAuthAddPhoneBrowserInput) (*CodexOAuthAddPhoneBrowserOutput, error) {
	cfg := s.codexOAuthSettings(ctx)
	label := cfg.label(input.GetLabel())
	output := &CodexOAuthAddPhoneBrowserOutput{PhoneReuseCount: input.GetPhone().GetReuseCount(), PhoneReuseLimit: input.GetPhone().GetReuseLimit()}
	data := codexOAuthBrowserData(label, input.GetPhone())
	step := s.activityStep(ctx, input.GetJobId(), contracts.StepCodexOAuthBrowserAddPhone, false, true)
	_, err := step.run(func() (activityStepResult, error) {
		stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), contracts.StepCodexOAuthBrowserAddPhone, "handling codex oauth add phone", data.messageData())
		defer stopHeartbeat()
		account, err := s.codexOAuthBrowserAccount(ctx, input.GetAccountId())
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		flow, err := s.newCodexOAuthBrowserSessionFlow(ctx, account, input.GetJobId(), label, input.GetPhone(), cfg, input.GetAllowAddPhone(), false, input.GetSession(), data, contracts.StepCodexOAuthBrowserAddPhone)
		if err != nil {
			data.setError(err)
			return data.messageData(), err
		}
		addPhoneResult, err := flow.handleAddPhoneStage()
		if err != nil {
			flow.releasePhoneOnFailure()
			output.AddPhoneRequired = addPhoneResult.addPhoneRequired
			output.ErrorMessage = err.Error()
			data.setError(err)
			return data.messageData(), err
		}
		result := flow.result()
		output.Success = true
		output.AddPhoneConfirmed = result.addPhoneConfirmed
		output.AddPhoneRequired = result.addPhoneRequired
		output.PhoneReuseCount = result.phoneReuseCount
		output.PhoneReuseLimit = result.phoneReuseLimit
		output.Data = data.messageData()
		return data.messageData(), nil
	})
	if output.Data == nil {
		output.Data = data.messageData()
	}
	return output, err
}
