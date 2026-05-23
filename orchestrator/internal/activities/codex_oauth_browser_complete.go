package activities

import (
	"context"
	"fmt"
	"strings"
)

func (s *Server) CodexOAuthCompleteBrowserActivity(ctx context.Context, input CodexOAuthCompleteBrowserInput) (CodexOAuthCompleteBrowserOutput, error) {
	cfg := s.codexOAuthConfig.withDefaults()
	label := cfg.label(input.GetLabel())
	output := CodexOAuthCompleteBrowserOutput{}
	data := codexOAuthBrowserData(label, nil)
	data["auth_secret_written"] = false
	data["account_auth_written"] = false
	data["callback_url_captured"] = false
	step := s.activityStep(ctx, input.GetJobId(), stepCodexOAuthBrowserComplete, false, true)
	_, err := step.run(func() (any, error) {
		stopHeartbeat := startActivityHeartbeat(ctx, input.GetJobId(), stepCodexOAuthBrowserComplete, "completing codex oauth browser", data)
		defer stopHeartbeat()
		account, err := s.codexOAuthBrowserAccount(ctx, input.GetAccountId())
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		flow, err := s.newCodexOAuthBrowserSessionFlow(ctx, account, input.GetJobId(), label, nil, cfg, false, input.GetMarkPhoneConfirmedOnSuccess(), input.GetSession(), data)
		if err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		verifier := s.loadRuntimeSecret(ctx, input.GetSession().GetPkceSecretKey())
		if strings.TrimSpace(verifier) == "" {
			err := fmt.Errorf("codex oauth pkce verifier is missing")
			data["error_message"] = err.Error()
			return data, err
		}
		flow.pkce = codexOAuthPKCE{verifier: verifier}
		if err := flow.completeAuthorization(); err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		if err := flow.persistAuthorization(); err != nil {
			data["error_message"] = err.Error()
			return data, err
		}
		output.Success = true
		output.AuthSecretKey = flow.secretKey
		output.Data = protoData(data)
		return data, nil
	})
	if output.Data == nil {
		output.Data = protoData(data)
	}
	return output, err
}
