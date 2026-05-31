package api

import (
	"strings"

	"orchestrator/internal/contracts"
)

type n8nDynamicProxyProfile struct {
	Purpose              string
	ProtocolMode         string
	AuthEdgeCheckEnabled bool
	AuthEdgeCheckTarget  string
}

func (profile n8nDynamicProxyProfile) withAction(action contracts.ActionProfile) n8nDynamicProxyProfile {
	profile.Purpose = action.ActionID
	return profile
}

func n8nProbeProxyProfile() n8nDynamicProxyProfile {
	return (n8nDynamicProxyProfile{
		AuthEdgeCheckEnabled: true,
		AuthEdgeCheckTarget:  n8nAuthEdgeCheckTargetAuthSession,
	}).withAction(contracts.ResolveActionProfile(contracts.ActionProbeAccount))
}

func (profile n8nDynamicProxyProfile) normalized() n8nDynamicProxyProfile {
	profile.Purpose = strings.TrimSpace(profile.Purpose)
	profile.ProtocolMode = strings.TrimSpace(profile.ProtocolMode)
	profile.AuthEdgeCheckTarget = strings.TrimSpace(profile.AuthEdgeCheckTarget)
	return profile
}

func (profile n8nDynamicProxyProfile) authEdgeCheckEnabled() bool {
	return profile.AuthEdgeCheckEnabled
}

func n8nDynamicProxyProfileForAction(actionID string) (n8nDynamicProxyProfile, error) {
	profile, err := n8nRuntimeProfileForAction("dynamic proxy", actionID)
	if err != nil {
		return n8nDynamicProxyProfile{}, err
	}
	if profile.DynamicProxy == nil {
		return n8nDynamicProxyProfile{}, unsupportedN8NAuthActionError("dynamic proxy", profile.ActionID)
	}
	return *profile.DynamicProxy, nil
}
