package api

import (
	"orchestrator/internal/contracts"
	"orchestrator/pb"
)

type n8nProtocolAuthProfileDefinition struct {
	Mode               string
	ResultSecretPrefix string
	StartStep          string
	WaitStep           string
	CompleteStep       string
	Failure            n8nAuthFailureConfig
	OTP                n8nAuthOTPDefinition
	MissingTokenError  string
	RequireAccess      bool
	IncludePlusTrial   bool
}

var (
	n8nRegisterProtocolAuthDefinition = n8nProtocolAuthProfileDefinition{
		Mode:               protocolRegisterMode,
		ResultSecretPrefix: n8nRegisterProtocolResultSecretPrefix,
		StartStep:          contracts.StepRegisterAccountProtocolStart,
		WaitStep:           contracts.StepRegisterAccountProtocol,
		CompleteStep:       contracts.StepRegisterAccountProtocolComplete,
		Failure:            n8nRegisterAuthFailure(),
		OTP: n8nAuthOTPDefinition{
			StepName:             contracts.StepRegisterAccountProtocolOTPWait,
			ResumeSecretPrefix:   n8nRegisterProtocolResumeURLSecretPrefix,
			FlowIDParam:          registerProtocolFlowIDParam,
			IssuedAfterParam:     registerProtocolOTPIssuedAfterParam,
			TimeoutParam:         registerProtocolOTPTimeoutParam,
			ResumeSecretKeyParam: registerProtocolOTPResumeSecretKeyParam,
			PollReason:           "gpt_register_protocol_otp",
		},
		MissingTokenError: "protocol register did not return ChatGPT access token",
		RequireAccess:     true,
		IncludePlusTrial:  true,
	}

	n8nLoginProtocolAuthDefinition = n8nProtocolAuthProfileDefinition{
		Mode:               protocolLoginMode,
		ResultSecretPrefix: n8nLoginProtocolResultSecretPrefix,
		StartStep:          contracts.StepLoginSessionProtocolStart,
		WaitStep:           contracts.StepLoginSessionProtocol,
		CompleteStep:       contracts.StepLoginSessionProtocolComplete,
		OTP: n8nAuthOTPDefinition{
			StepName:             contracts.StepLoginSessionProtocolOTPWait,
			ResumeSecretPrefix:   n8nLoginProtocolResumeURLSecretPrefix,
			FlowIDParam:          loginProtocolFlowIDParam,
			IssuedAfterParam:     loginProtocolOTPIssuedAfterParam,
			TimeoutParam:         loginProtocolOTPTimeoutParam,
			ResumeSecretKeyParam: loginProtocolOTPResumeSecretKeyParam,
			PollReason:           "gpt_login_session_protocol_otp",
		},
		MissingTokenError: "protocol login did not return ChatGPT access token",
		RequireAccess:     true,
	}
)

func n8nRegisterProtocolActionProfile() n8nRegisterProtocolAuthProfile {
	profile := contracts.ResolveActionProfile(contracts.ActionRegisterProtocol)
	protocol := n8nRegisterProtocolAuthDefinition
	return n8nRegisterProtocolAuthProfile{
		Start: n8nRegisterJobConfig{
			n8nActionJobConfig: (n8nActionJobConfig{
				EmailParam: registerProtocolEmailParam,
			}).withAction(profile),
			Params: func(accountID string, req *pb.RegisterAccountRequest) map[string]string {
				params := map[string]string{"account_id": accountID}
				putProtocolGeoParams(params, req.GetCountryCode(), req.GetRegion())
				return params
			},
		},
		Protocol: protocol.profile(profile),
		Proxy:    protocol.proxyProfile(profile),
	}
}

func (profile n8nRegisterProtocolAuthProfile) runtimeProfile() n8nActionRuntimeProfile {
	return n8nActionRuntimeProfile{
		RegisterStart: n8nRuntimeProfilePtr(profile.Start),
		ProtocolAuth: n8nRuntimeProfilePtr(n8nProtocolAuthRuntimeProfile{
			Protocol: profile.Protocol,
			Proxy:    profile.Proxy,
		}),
		DynamicProxy: n8nRuntimeProfilePtr(profile.Proxy),
	}
}

func n8nLoginSessionProtocolActionProfile() n8nLoginSessionProtocolAuthProfile {
	profile := contracts.ResolveActionProfile(contracts.ActionLoginSessionProtocol)
	protocol := n8nLoginProtocolAuthDefinition
	return n8nLoginSessionProtocolAuthProfile{
		Start: (n8nActionJobConfig{
			EmailParam:             loginProtocolEmailParam,
			TargetConnectivityURLs: n8nAuthTargetConnectivityURL,
		}).withAction(profile),
		Protocol: protocol.profile(profile),
		Proxy:    protocol.proxyProfile(profile),
	}
}

func (profile n8nLoginSessionProtocolAuthProfile) runtimeProfile() n8nActionRuntimeProfile {
	return n8nActionRuntimeProfile{
		LoginStart: n8nRuntimeProfilePtr(profile.Start),
		ProtocolAuth: n8nRuntimeProfilePtr(n8nProtocolAuthRuntimeProfile{
			Protocol: profile.Protocol,
			Proxy:    profile.Proxy,
		}),
		DynamicProxy: n8nRuntimeProfilePtr(profile.Proxy),
	}
}

func (definition n8nProtocolAuthProfileDefinition) profile(profile contracts.ActionProfile) n8nProtocolAuthProfile {
	return n8nProtocolAuthProfile{
		Lifecycle: n8nProtocolAuthLifecycleConfig{
			Mode:               definition.Mode,
			ResultSecretPrefix: definition.ResultSecretPrefix,
			StartStep:          definition.StartStep,
			WaitStep:           definition.WaitStep,
			CompleteStep:       definition.CompleteStep,
			Failure:            definition.Failure,
		},
		OTP: definition.OTP.waitConfig(),
		Finish: (n8nAuthFinishConfig{
			Driver:             "protocol",
			Mode:               definition.Mode,
			ResultSecretPrefix: definition.ResultSecretPrefix,
			CompleteStep:       definition.CompleteStep,
			MissingTokenError:  definition.MissingTokenError,
			RequireAccess:      definition.RequireAccess,
			IncludePlusTrial:   definition.IncludePlusTrial,
		}).withAction(profile),
		Fail: (n8nAuthFailConfig{
			Mode:                definition.Mode,
			FailureStepFallback: definition.StartStep,
			ProtocolFlowParam:   definition.OTP.FlowIDParam,
			Failure:             definition.Failure,
		}).withAction(profile),
	}
}

func (definition n8nProtocolAuthProfileDefinition) proxyProfile(profile contracts.ActionProfile) n8nDynamicProxyProfile {
	return (n8nDynamicProxyProfile{
		ProtocolMode:         definition.Mode,
		AuthEdgeCheckEnabled: true,
		AuthEdgeCheckTarget:  n8nAuthEdgeCheckTargetCSRF,
	}).withAction(profile)
}
