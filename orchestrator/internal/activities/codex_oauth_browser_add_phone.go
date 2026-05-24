package activities

import "context"

func (s *Server) CodexOAuthAddPhoneBrowserActivity(ctx context.Context, input CodexOAuthAddPhoneBrowserInput) (CodexOAuthAddPhoneBrowserOutput, error) {
	cfg := s.codexOAuthConfig.withDefaults()
	label := cfg.label(input.GetLabel())
	output := CodexOAuthAddPhoneBrowserOutput{PhoneReuseCount: input.GetPhone().GetReuseCount(), PhoneReuseLimit: input.GetPhone().GetReuseLimit()}
	data := codexOAuthBrowserData(label, input.GetPhone())
	step := s.activityStep(ctx, input.GetJobId(), stepCodexOAuthBrowserAddPhone, false, true)
	_, err := step.run(func() (any, error) {
		stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepCodexOAuthBrowserAddPhone, "handling codex oauth add phone", data)
		defer stopHeartbeat()
		account, err := s.codexOAuthBrowserAccount(ctx, input.GetAccountId())
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		flow, err := s.newCodexOAuthBrowserSessionFlow(ctx, account, input.GetJobId(), label, input.GetPhone(), cfg, input.GetAllowAddPhone(), false, input.GetSession(), data, stepCodexOAuthBrowserAddPhone)
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		addPhoneResult, err := flow.handleAddPhoneStage()
		if err != nil {
			flow.releasePhoneOnFailure()
			output.AddPhoneRequired = addPhoneResult.addPhoneRequired
			output.ErrorMessage = err.Error()
			data["error_message"] = err.Error()
			return data, err
		}
		result := flow.result()
		output.Success = true
		output.AddPhoneConfirmed = result.addPhoneConfirmed
		output.AddPhoneRequired = result.addPhoneRequired
		output.PhoneReuseCount = result.phoneReuseCount
		output.PhoneReuseLimit = result.phoneReuseLimit
		output.Data = protoData(data)
		return data, nil
	})
	if output.Data == nil {
		output.Data = protoData(data)
	}
	return output, err
}
