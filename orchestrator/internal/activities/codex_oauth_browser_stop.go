package activities

import (
	"context"
	"strings"

	"orchestrator/pb"
)

func (s *Server) CodexOAuthStopBrowserActivity(ctx context.Context, input CodexOAuthStopBrowserInput) error {
	session := input.GetSession()
	if session == nil {
		return nil
	}
	if s.browserAutomationClient != nil && strings.TrimSpace(session.GetSessionId()) != "" {
		flow := newBrowserAuthFlow(codexOAuthBrowserMode, input.GetJobId(), &pb.Account{}, "")
		flow.flowID = strings.TrimSpace(session.GetFlowId())
		flow.sessionID = strings.TrimSpace(session.GetSessionId())
		flow.stopSession(s.browserAutomationClient)
	}
	return s.deleteRuntimeSecret(ctx, session.GetPkceSecretKey())
}
