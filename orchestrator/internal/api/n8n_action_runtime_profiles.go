package api

import "orchestrator/internal/contracts"

type n8nCodexOAuthFlowKind string

const (
	n8nCodexOAuthBrowserFlow  n8nCodexOAuthFlowKind = "browser"
	n8nCodexOAuthProtocolFlow n8nCodexOAuthFlowKind = "protocol"
)

type n8nActionRuntimeProfile struct {
	ActionID string

	RegisterStart  *n8nRegisterJobConfig
	LoginStart     *n8nActionJobConfig
	BrowserAuth    *n8nBrowserAuthProfile
	ProtocolAuth   *n8nProtocolAuthRuntimeProfile
	CodexOAuth     *n8nCodexOAuthProfile
	DynamicProxy   *n8nDynamicProxyProfile
	CodexOAuthFlow n8nCodexOAuthFlowKind
}

type n8nActionRuntimeProfileDefinition struct {
	ActionID string
	Build    func() n8nActionRuntimeProfile
}

var n8nActionRuntimeProfileDefinitions = []n8nActionRuntimeProfileDefinition{
	{
		ActionID: contracts.ActionRegister,
		Build: func() n8nActionRuntimeProfile {
			return n8nRegisterActionProfile().runtimeProfile()
		},
	},
	{
		ActionID: contracts.ActionLoginSession,
		Build: func() n8nActionRuntimeProfile {
			return n8nLoginSessionActionProfile().runtimeProfile()
		},
	},
	{
		ActionID: contracts.ActionRegisterProtocol,
		Build: func() n8nActionRuntimeProfile {
			return n8nRegisterProtocolActionProfile().runtimeProfile()
		},
	},
	{
		ActionID: contracts.ActionLoginSessionProtocol,
		Build: func() n8nActionRuntimeProfile {
			return n8nLoginSessionProtocolActionProfile().runtimeProfile()
		},
	},
	{
		ActionID: contracts.ActionProbeAccount,
		Build: func() n8nActionRuntimeProfile {
			return n8nActionRuntimeProfile{
				DynamicProxy: n8nRuntimeProfilePtr(n8nProbeProxyProfile()),
			}
		},
	},
}

func n8nRuntimeProfileForAction(kind string, actionID string) (n8nActionRuntimeProfile, error) {
	profile, ok := n8nRuntimeProfile(actionID)
	if !ok {
		return n8nActionRuntimeProfile{}, unsupportedN8NAuthActionError(kind, contracts.NormalizeActionID(actionID))
	}
	return profile, nil
}

func n8nRuntimeProfile(actionID string) (n8nActionRuntimeProfile, bool) {
	normalized := contracts.NormalizeActionID(actionID)
	for _, definition := range n8nActionRuntimeProfileDefinitions {
		if contracts.NormalizeActionID(definition.ActionID) != normalized {
			continue
		}
		profile := definition.Build()
		profile.ActionID = normalized
		return profile, true
	}
	if definition, ok := n8nCodexOAuthProfileDefinitionForAction(normalized); ok {
		profile := definition.runtimeProfile()
		profile.ActionID = normalized
		return profile, true
	}
	return n8nActionRuntimeProfile{}, false
}

func n8nRuntimeProfilePtr[T any](value T) *T {
	return &value
}
