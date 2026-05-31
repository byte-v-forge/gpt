package contracts

import "strings"

type ActionProfile struct {
	ActionID       string
	Label          string
	WorkflowLabel  string
	APILabel       string
	ResultLabel    string
	FailureMessage string
}

type ActionWorkflowStartProfile struct {
	ActionID      string
	WorkflowLabel string
	APILabel      string
}

var actionProfiles = []ActionProfile{
	{
		ActionID:       ActionRegister,
		Label:          "register",
		WorkflowLabel:  "register",
		APILabel:       "register",
		ResultLabel:    "register",
		FailureMessage: "register failed",
	},
	{
		ActionID:       ActionProbeAccount,
		Label:          "probe",
		WorkflowLabel:  "probe-account",
		APILabel:       "probe",
		FailureMessage: "probe account failed",
	},
	{
		ActionID:       ActionLoginSession,
		Label:          "login-session",
		WorkflowLabel:  "login-session",
		APILabel:       "login-session",
		ResultLabel:    "login",
		FailureMessage: "login session failed",
	},
	{
		ActionID:       ActionRegisterProtocol,
		Label:          "register-protocol",
		WorkflowLabel:  "register-protocol",
		APILabel:       "register-protocol",
		ResultLabel:    "register",
		FailureMessage: "register protocol failed",
	},
	{
		ActionID:       ActionLoginSessionProtocol,
		Label:          "login-session-protocol",
		WorkflowLabel:  "login-session-protocol",
		APILabel:       "login-session-protocol",
		ResultLabel:    "login",
		FailureMessage: "login session protocol failed",
	},
	{
		ActionID:       ActionCodexOAuth,
		Label:          "codex oauth",
		WorkflowLabel:  "codex-oauth",
		APILabel:       "codex-oauth",
		FailureMessage: "codex oauth failed",
	},
	{
		ActionID:       ActionCodexOAuthProtocol,
		Label:          "codex oauth protocol",
		WorkflowLabel:  "codex-oauth-protocol",
		APILabel:       "codex-oauth-protocol",
		FailureMessage: "codex oauth protocol failed",
	},
	{
		ActionID:       ActionCodexOAuthAddPhone,
		Label:          "codex oauth add-phone",
		WorkflowLabel:  "codex-oauth-add-phone",
		APILabel:       "codex-oauth-add-phone",
		FailureMessage: "codex oauth add phone failed",
	},
	{
		ActionID:       ActionCodexOAuthBatchAddPhone,
		Label:          "codex oauth batch",
		WorkflowLabel:  "codex-oauth-batch-add-phone",
		APILabel:       "codex-oauth-batch-add-phone",
		FailureMessage: "codex oauth batch add phone failed",
	},
}

func ActionProfiles() []ActionProfile {
	profiles := make([]ActionProfile, len(actionProfiles))
	copy(profiles, actionProfiles)
	return profiles
}

func LookupActionProfile(actionID string) (ActionProfile, bool) {
	actionID = NormalizeActionID(actionID)
	for _, profile := range actionProfiles {
		if strings.EqualFold(profile.ActionID, actionID) {
			return profile, true
		}
	}
	return ActionProfile{}, false
}

func ResolveActionProfile(actionID string) ActionProfile {
	profile, ok := LookupActionProfile(actionID)
	if ok {
		return profile
	}
	return ActionProfile{ActionID: NormalizeActionID(actionID)}
}

func NormalizeActionID(actionID string) string {
	return strings.ToUpper(strings.TrimSpace(actionID))
}

func WorkflowLabelForAction(actionID string) string {
	return ResolveActionProfile(actionID).WorkflowLabelOrDefault()
}

func (profile ActionProfile) WorkflowStartProfile() ActionWorkflowStartProfile {
	return ActionWorkflowStartProfile{
		ActionID:      profile.ActionID,
		WorkflowLabel: profile.WorkflowLabelOrDefault(),
		APILabel:      profile.APILabelOrDefault(),
	}
}

func (profile ActionProfile) WorkflowLabelOrDefault() string {
	return firstNonEmpty(profile.WorkflowLabel, profile.APILabel, profile.Label, profile.ActionID)
}

func (profile ActionProfile) APILabelOrDefault() string {
	return firstNonEmpty(profile.APILabel, profile.WorkflowLabel, profile.Label, profile.ActionID)
}

func (profile ActionProfile) ResultLabelOrDefault() string {
	return firstNonEmpty(profile.ResultLabel, profile.Label, profile.WorkflowLabel, profile.ActionID)
}

func (profile ActionProfile) FailureMessageOrDefault() string {
	if message := firstNonEmpty(profile.FailureMessage); message != "" {
		return message
	}
	if label := firstNonEmpty(profile.Label, profile.ActionID); label != "" {
		return label + " failed"
	}
	return "action failed"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
