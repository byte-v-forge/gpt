package api

import (
	"orchestrator/internal/contracts"
	"orchestrator/pb"
)

type n8nBrowserAuthProfileDefinition struct {
	Mode               string
	ResultSecretPrefix string
	StartStep          string
	CompleteStep       string
	Failure            n8nAuthFailureConfig
	OTP                n8nAuthOTPDefinition
	MissingTokenError  string
	RequireSession     bool
	RequireAccess      bool
	IncludePlusTrial   bool
}

var (
	n8nRegisterBrowserAuthDefinition = n8nBrowserAuthProfileDefinition{
		Mode:               protocolRegisterMode,
		ResultSecretPrefix: n8nRegisterResultSecretPrefix,
		StartStep:          contracts.StepRegisterAccountStart,
		CompleteStep:       contracts.StepRegisterAccountComplete,
		Failure:            n8nRegisterAuthFailure(),
		OTP: n8nAuthOTPDefinition{
			StepName:             contracts.StepRegisterAccountOTPWait,
			ResumeSecretPrefix:   n8nRegisterResumeURLSecretPrefix,
			FlowIDParam:          registerFlowIDParam,
			IssuedAfterParam:     registerOTPIssuedAfterParam,
			TimeoutParam:         registerOTPTimeoutParam,
			ResumeSecretKeyParam: registerOTPResumeSecretKeyParam,
			PollReason:           "gpt_register_otp",
		},
		MissingTokenError: "browser register did not return both ChatGPT session and access tokens",
		RequireSession:    true,
		RequireAccess:     true,
		IncludePlusTrial:  true,
	}

	n8nLoginBrowserAuthDefinition = n8nBrowserAuthProfileDefinition{
		Mode:               protocolLoginMode,
		ResultSecretPrefix: n8nLoginSessionResultSecretPrefix,
		StartStep:          contracts.StepLoginSessionStart,
		CompleteStep:       contracts.StepLoginSessionComplete,
		OTP: n8nAuthOTPDefinition{
			StepName:             contracts.StepLoginSessionOTPWait,
			ResumeSecretPrefix:   n8nLoginSessionResumeURLSecretPrefix,
			FlowIDParam:          loginSessionFlowIDParam,
			IssuedAfterParam:     loginSessionOTPIssuedAfterParam,
			TimeoutParam:         loginSessionOTPTimeoutParam,
			ResumeSecretKeyParam: loginSessionOTPResumeSecretKeyParam,
			PollReason:           "gpt_login_session_otp",
		},
		MissingTokenError: "browser login did not return both ChatGPT session and access tokens",
		RequireSession:    true,
		RequireAccess:     true,
	}
)

func n8nRegisterActionProfile() n8nRegisterAuthProfile {
	profile := contracts.ResolveActionProfile(contracts.ActionRegister)
	return n8nRegisterAuthProfile{
		Start: n8nRegisterJobConfig{
			n8nActionJobConfig: (n8nActionJobConfig{
				EmailParam:             registerEmailParam,
				TargetConnectivityURLs: n8nAuthTargetConnectivityURL,
			}).withAction(profile),
			GenerateFingerprint: true,
			Params: func(accountID string, req *pb.RegisterAccountRequest) map[string]string {
				return registerAccountJobParams(accountID, req.GetOtpOptions(), req.GetCountryCode(), req.GetRegion())
			},
		},
		Browser: n8nRegisterBrowserAuthDefinition.profile(profile),
	}
}

func (profile n8nRegisterAuthProfile) runtimeProfile() n8nActionRuntimeProfile {
	return n8nActionRuntimeProfile{
		RegisterStart: n8nRuntimeProfilePtr(profile.Start),
		BrowserAuth:   n8nRuntimeProfilePtr(profile.Browser),
	}
}

func n8nLoginSessionActionProfile() n8nLoginSessionAuthProfile {
	profile := contracts.ResolveActionProfile(contracts.ActionLoginSession)
	return n8nLoginSessionAuthProfile{
		Start: (n8nActionJobConfig{
			EmailParam:             loginSessionEmailParam,
			TargetConnectivityURLs: n8nAuthTargetConnectivityURL,
		}).withAction(profile),
		Browser: n8nLoginBrowserAuthDefinition.profile(profile),
	}
}

func (profile n8nLoginSessionAuthProfile) runtimeProfile() n8nActionRuntimeProfile {
	return n8nActionRuntimeProfile{
		LoginStart:  n8nRuntimeProfilePtr(profile.Start),
		BrowserAuth: n8nRuntimeProfilePtr(profile.Browser),
	}
}

func (definition n8nBrowserAuthProfileDefinition) profile(profile contracts.ActionProfile) n8nBrowserAuthProfile {
	return n8nBrowserAuthProfile{
		Lifecycle: n8nBrowserAuthLifecycleConfig{
			Mode:               definition.Mode,
			ResultSecretPrefix: definition.ResultSecretPrefix,
			StartStep:          definition.StartStep,
			CompleteStep:       definition.CompleteStep,
			ResultLabel:        profile.ResultLabelOrDefault(),
			Failure:            definition.Failure,
		},
		OTP: definition.OTP.waitConfig(),
		Finish: (n8nAuthFinishConfig{
			Driver:             "browser",
			Mode:               definition.Mode,
			ResultSecretPrefix: definition.ResultSecretPrefix,
			CompleteStep:       definition.CompleteStep,
			MissingTokenError:  definition.MissingTokenError,
			RequireSession:     definition.RequireSession,
			RequireAccess:      definition.RequireAccess,
			IncludePlusTrial:   definition.IncludePlusTrial,
		}).withAction(profile),
		Fail: (n8nAuthFailConfig{
			Mode:                definition.Mode,
			FailureStepFallback: definition.StartStep,
			BrowserCancel:       true,
			Failure:             definition.Failure,
		}).withAction(profile),
	}
}
